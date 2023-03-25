package main

import (
	"context"
	"fmt"
	"log"

	// "log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/mafredri/cdp"
	"github.com/mafredri/cdp/devtool"
	"github.com/mafredri/cdp/protocol/debugger"
	"github.com/mafredri/cdp/protocol/runtime"

	// "github.com/mafredri/cdp/protocol/runtime"
	"github.com/mafredri/cdp/rpcc"
)


var client *cdp.Client
var pausedClient debugger.PausedClient
var wg sync.WaitGroup
var defaultObjects map[string]string
var objectCount int = 0
var stackIDs map[string]string
var stackIdToResourceIds map[string][]string // stackID to resourcesID
var IdConditionCount int
var stackToLogicalIds map[string][]string // stackID to logicalIds
var resourceIdToType map[string]string
var stackIdToClassname map[string]string

func run(timeout time.Duration) error {
	// ctx, cancel := context.WithTimeout(context.Background(), timeout)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Use the DevTools HTTP/JSON API to manage targets (e.g. pages, webworkers).
	devt := devtool.New("http://localhost:2334")
	pt, err := devt.Get(ctx, devtool.Node)
	if err != nil {
		return err
	}

	fmt.Println(pt.WebSocketDebuggerURL)
	// Initiate a new RPC connection to the Chrome DevTools Protocol target.
	conn, err := rpcc.DialContext(ctx, pt.WebSocketDebuggerURL)
	if err != nil {
		return err
	}
	defer conn.Close() // Leaving connections open will leak memory.
	client = cdp.NewClient(conn)
	
	// Enable debugging features
	_, err = client.Debugger.Enable(ctx, &debugger.EnableArgs{})
	if err != nil {
		fmt.Print("\n Error while enbling debugger ",err)
		return err
	}

	curDir, _ := os.Getwd()
	parDir := filepath.Dir(curDir)

	// urlRegex := "^.*" + parDir + "/my-project2/node_modules/constructs/lib/construct.js$"
	// columnNumber := 9
	// client.Debugger.SetBreakpointByURL(ctx, &debugger.SetBreakpointByURLArgs{
	// 	URLRegex:     &urlRegex,
	// 	LineNumber:   368,
	// 	ColumnNumber: &columnNumber,
	// })
	// "/my-project2/node_modules/ts-node/dist/bin.js$"
	// urlRegex := "^.*" + parDir + "/my-project2/node_modules/constructs/lib/construct.js$"
	// columnNumber :=0
	// client.Debugger.SetBreakpointByURL(ctx, &debugger.SetBreakpointByURLArgs{
	// 	URLRegex:     &urlRegex,
	// 	LineNumber:   131,
	// 	ColumnNumber: &columnNumber,
	// })

	// use this for Tapping get children
	urlRegex := "^.*" + parDir + "/my-project2/node_modules/constructs/lib/construct.js$"
	columnNumber := 8
	client.Debugger.SetBreakpointByURL(ctx, &debugger.SetBreakpointByURLArgs{
		URLRegex:     &urlRegex,
		LineNumber:   129,
		ColumnNumber: &columnNumber,
	})
	
	pausedClient, err = client.Debugger.Paused(ctx)
	client.Debugger.Resume(ctx, &debugger.ResumeArgs{})

	//used to continue program execution if the program is currently paused waiting for a debugger to attach.
	//If the program is not currently waiting for a debugger, this method will simply return immediately without doing anything.
	err = client.Runtime.RunIfWaitingForDebugger(ctx)
	if err != nil {
		return err
	}

	IdConditionCount = 0

	wg.Add(1)
	go parseResourceDataBreakpointData(ctx,conn)

	wg.Wait()
	return nil
}

func setDefaultObjects(dataObj *runtime.RemoteObjectID, ctx context.Context) int {
	defaultObjectsLocal := make(map[string]string)

	data, _ := client.Runtime.GetProperties(ctx, &runtime.GetPropertiesArgs{ObjectID: *dataObj})
	objectCount = 0
	stackIDs = make(map[string]string)
	for _, dataObject := range data.Result {
		if *dataObject.IsOwn == true && dataObject.Name != "Tree" {
			fmt.Print("\n NAme ", dataObject.Name, " is stackNAme")
			stackIDs["\""+dataObject.Name+"\""] = "\"" + dataObject.Name + "\""
		} else {
			defaultObjectsLocal[dataObject.Name] = dataObject.Name
			fmt.Print("\n NAme ", dataObject.Name, " is Default")
			objectCount++
		}
	}
	defaultObjects = defaultObjectsLocal
	return objectCount

}
func getLogicalIds(dataObj *runtime.RemoteObjectID, ctx context.Context) []string {
	var logicalIds []string
	LogicalIdObjects, _ := client.Runtime.GetProperties(ctx, &runtime.GetPropertiesArgs{ObjectID: *dataObj})
	for _, logicalIdObject := range LogicalIdObjects.Result {
		if logicalIdObject.Name == "reverse" {
			reverseObjects, _ := client.Runtime.GetProperties(ctx, &runtime.GetPropertiesArgs{ObjectID: *logicalIdObject.Value.ObjectID})
			for _, reverseObject := range reverseObjects.Result {
				if *reverseObject.IsOwn == true && reverseObject.Name != "CDKMetadata" {
					logicalIds = append(logicalIds, reverseObject.Name)
				}
			}
		}
	}
	return logicalIds
}

func getResourceType(dataObj *runtime.RemoteObjectID, ctx context.Context) string {
	resourceObjects, _ := client.Runtime.GetProperties(ctx, &runtime.GetPropertiesArgs{ObjectID: *dataObj})

	for _, resourceObject := range resourceObjects.Result {
		if resourceObject.Name == "_resource" {
			cfnResourceObjects, _ := client.Runtime.GetProperties(ctx, &runtime.GetPropertiesArgs{ObjectID: *resourceObject.Value.ObjectID})
			for _, cfnResourceObject := range cfnResourceObjects.Result {
				if cfnResourceObject.Name == "cfnResourceType" {
					return cfnResourceObject.Value.String()
				}
				// we can get path also from here by
				if cfnResourceObject.Name == "cfnProperties" {
					// get cdkpath
					//its inside cfnOptions -> metadata -> aws:cdk:path
				}
			}
		}
	}
	fmt.Print("\n no type found ")
	return ""
}

func getResources(dataObj *runtime.RemoteObjectID, ctx context.Context) {
	nodeObjects, _ := client.Runtime.GetProperties(ctx, &runtime.GetPropertiesArgs{ObjectID: *dataObj})
	var resourceIds []string
	var idIndex int
	for index, nodeObject := range nodeObjects.Result {
		if nodeObject.Name == "_children" {
			childrenObjects, _ := client.Runtime.GetProperties(ctx, &runtime.GetPropertiesArgs{ObjectID: *nodeObject.Value.ObjectID})
			for _, childrenObject := range childrenObjects.Result {
				if *childrenObject.IsOwn == true && childrenObject.Name != "CDKMetadata" {
					resourceObjectId := childrenObject.Value.ObjectID
					resourceType := getResourceType(resourceObjectId, ctx)
					resourceIdToType = make(map[string]string)
					resourceIdToType[childrenObject.Name] = resourceType
					resourceIds = append(resourceIds, childrenObject.Name)
				}
			}

		} else if nodeObject.Name == "id" {
			idIndex = index
		}
	}
	stackIdToResourceIds = make(map[string][]string)
	stackIdToResourceIds[nodeObjects.Result[idIndex].Value.String()] = resourceIds
	fmt.Print("classname of stack is  ")
}

func getStackData(dataObj *runtime.RemoteObjectID, ctx context.Context) {
	stackDataObjects, _ := client.Runtime.GetProperties(ctx, &runtime.GetPropertiesArgs{ObjectID: *dataObj})
	var logicalIds []string
	var stackName string
	for _, stackDataObject := range stackDataObjects.Result {
		if stackDataObject.Name == "_logicalIds" {
			logicalIdObjectId := stackDataObject.Value.ObjectID
			logicalIds = getLogicalIds(logicalIdObjectId, ctx)
		} else if stackDataObject.Name == "node" {
			nodeObjectID := stackDataObject.Value.ObjectID
			getResources(nodeObjectID, ctx)
		} else if stackDataObject.Name == "_stackName" {
			stackToLogicalIds = make(map[string][]string)
			stackName = stackDataObject.Value.String()
		}
	}
	stackToLogicalIds[stackName] = logicalIds
}

func getChildrenData(dataObj *runtime.RemoteObjectID, ctx context.Context) {
	dataObjects, _ := client.Runtime.GetProperties(ctx, &runtime.GetPropertiesArgs{ObjectID: *dataObj})
	for _, dataObject := range dataObjects.Result {
		_, inDefaultObject := defaultObjects[dataObject.Name]
		_, inStackIds := stackIDs["\""+dataObject.Name+"\""]
		// stackID has "" at start and end so must add those for checking

		// stackId is present, its not default object and isOwn
		if !inDefaultObject && *dataObject.IsOwn == true && inStackIds {
			fmt.Print("\n object name being checked ", dataObject.Name)
			fmt.Print("\n ClassNAme of stacks = ", *dataObject.Value.ClassName)
			stackIdToClassname = make(map[string]string)
			stackIdToClassname[dataObject.Name] = *dataObject.Value.ClassName
			stackObject := dataObject.Value.ObjectID
			fmt.Print("\n stackResource maps ", stackIdToResourceIds)
			getStackData(stackObject, ctx)
		}
	}
}

func parseResourceDataBreakpointData(ctx context.Context , conn *rpcc.Conn) error {
	defer wg.Done()
	// getting current callstack and vars from the debugger process
	ev, err := pausedClient.Recv()
	if err != nil {
		return err
	}

	fmt.Println(ev.Reason)
	if ev.Reason == "other" {
		for _, callFrame := range ev.CallFrames {
			if callFrame.FunctionName == "get children" {
				Node, err := client.Runtime.GetProperties(ctx, &runtime.GetPropertiesArgs{ObjectID: *callFrame.This.ObjectID})
				if err != nil {
					fmt.Print("\n Error in accessing the this Object in Node")
				}
				var idIndex int
				var childrenIndex int
				for index, insideNode := range Node.Result {
					if insideNode.Name == "id" && insideNode.Value != nil {
						if insideNode.Value.String() == "\"Condition\"" {
							IdConditionCount++
						}
						idIndex = index
					}
					// LogicalIDs should be present
					if IdConditionCount >= 3 && insideNode.Name == "_children" {

						childrenIndex = index
					}
				}
				// stackID found
				if Node.Result[idIndex].Value.String() == "\"\"" && (len(stackIDs) != 0) && IdConditionCount >= 3 {
					fmt.Print("\n fetching children data for ID = ", Node.Result[idIndex].Value.String())
					getChildrenData(Node.Result[childrenIndex].Value.ObjectID, ctx)
				
				// Fetches All the stackIds
				} else if Node.Result[idIndex].Value.String() == "\"\"" && (len(stackIDs) == 0) && IdConditionCount >= 3 { // Getting StackIDs when id = ""
					objectCount = setDefaultObjects(Node.Result[childrenIndex].Value.ObjectID, ctx)
					fmt.Print("\n collected StackIDS ", stackIDs)

				} else if Node.Result[idIndex].Value.String() == "\"BootstrapVersion\"" {
					fmt.Print("\n BOOTSTARP VERSION IS FOUND \n", Node.Result[idIndex].Value.String())
					conn.Close();
				}
				fmt.Print("\n ID is ", Node.Result[idIndex].Value.String())

			}
		}
	}
	fmt.Print("\n stackIDs", stackIDs)
	fmt.Print("\n stackToLogicalIds ", stackToLogicalIds)
	fmt.Print("\n resourceToType ", resourceIdToType)
	fmt.Print("\n stackIdToresourceID ", stackIdToResourceIds)
	fmt.Print("\n ClasstypeToResourceID ", stackIdToClassname)

	client.Debugger.Resume(ctx, &debugger.ResumeArgs{})
	wg.Add(1)
	go parseResourceDataBreakpointData(ctx,conn)
	return nil
}

func main() {
	fmt.Print(PrepareMappings())
	log.Fatal("YES")
	// err := run(30000 * time.Second)
	// if err != nil {
	// 	log.Fatal(err)
	// }
}
