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
	"github.com/mafredri/cdp/rpcc"
)


var client *cdp.Client
var pausedClient debugger.PausedClient
var wg sync.WaitGroup
var stackIdToResourceIds map[string][]string // stackID to resourcesID
var stackToLogicalIds map[string][]string  // stackID to logicalIds
var resourceIdToType map[string]string
var stackToResourceToPath map[string] map[string ] string
var resourceIdToPath map[string] string 
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
	// urlRegex := "^.*" + parDir + "/my-project2/node_modules/constructs/lib/construct.js$"
	// columnNumber := 8
	// client.Debugger.SetBreakpointByURL(ctx, &debugger.SetBreakpointByURLArgs{
	// 	URLRegex:     &urlRegex,
	// 	LineNumber:   129,
	// 	ColumnNumber: &columnNumber,
	// })
	urlRegex := "^.*" + parDir + "/my-project2/node_modules/constructs/lib/construct.js$"
	columnNumber := 8
	client.Debugger.SetBreakpointByURL(ctx, &debugger.SetBreakpointByURLArgs{
		URLRegex:     &urlRegex,
		LineNumber:   367,
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


	wg.Add(1)
	go parseResourceDataBreakpointData(ctx,conn)

	
	wg.Wait()
	

	return nil
}

func getLogicalIds(dataObj *runtime.RemoteObjectID, ctx context.Context) []string {
	var logicalIds []string
	LogicalIdObjects, _ := client.Runtime.GetProperties(ctx, &runtime.GetPropertiesArgs{ObjectID: *dataObj})
	for _, logicalIdObject := range LogicalIdObjects.Result {
		if logicalIdObject.Name == "reverse" {
			reverseObjects, _ := client.Runtime.GetProperties(ctx, &runtime.GetPropertiesArgs{ObjectID: *logicalIdObject.Value.ObjectID})
			for _, reverseObject := range reverseObjects.Result {
				if *reverseObject.IsOwn && reverseObject.Name != "CDKMetadata" {
					logicalIds = append(logicalIds, reverseObject.Name)
				}
			}
		}
	}
	return logicalIds
}

func GetCdkPath(CfnOptionsObjectId *runtime.RemoteObjectID,ctx context.Context) string {
	cfnOptionsData,errCfn := client.Runtime.GetProperties(ctx, &runtime.GetPropertiesArgs{ObjectID: *CfnOptionsObjectId})
	if errCfn != nil {
		fmt.Print("\n Not able to get CfnOptions from callstackData")
	}
	for _,insideCfnOptionData := range cfnOptionsData.Result{
		if insideCfnOptionData.Name == "metadata"{
			metaData, _ := client.Runtime.GetProperties(ctx, &runtime.GetPropertiesArgs{ObjectID: *insideCfnOptionData.Value.ObjectID})
			for _,insideMetaData := range metaData.Result{
				if insideMetaData.Name == "aws:cdk:path"{
					return insideMetaData.Value.String()
				}
			}
		}
	}
	return ""
}

func getResourceTypeAndPath(dataObj *runtime.RemoteObjectID, ctx context.Context) (string,string) {
	resourceObjects, _ := client.Runtime.GetProperties(ctx, &runtime.GetPropertiesArgs{ObjectID: *dataObj})
	resourceType := ""
	AwsCdkPath := ""
	for _, resourceObject := range resourceObjects.Result {
		if resourceObject.Name == "_resource" {
			cfnResourceObjects, _ := client.Runtime.GetProperties(ctx, &runtime.GetPropertiesArgs{ObjectID: *resourceObject.Value.ObjectID})
			for _, cfnResourceObject := range cfnResourceObjects.Result {
				if cfnResourceObject.Name == "cfnResourceType" {
					resourceType = cfnResourceObject.Value.String()
				}
				if cfnResourceObject.Name == "cfnOptions" {
					// get cdkpath
					AwsCdkPath = GetCdkPath(cfnResourceObject.Value.ObjectID,ctx)
					//its inside cfnOptions -> metadata -> aws:cdk:path
				}
			}
			return resourceType,AwsCdkPath
		}
	}
	return "",""
}

func getResources(dataObj *runtime.RemoteObjectID, ctx context.Context) {
	nodeObjects, _ := client.Runtime.GetProperties(ctx, &runtime.GetPropertiesArgs{ObjectID: *dataObj})
	var resourceIds []string
	var idIndex int
	for index, nodeObject := range nodeObjects.Result {
		if nodeObject.Name == "_children" {
			childrenObjects, _ := client.Runtime.GetProperties(ctx, &runtime.GetPropertiesArgs{ObjectID: *nodeObject.Value.ObjectID})
			for _, childrenObject := range childrenObjects.Result {
				if *childrenObject.IsOwn && childrenObject.Name != "CDKMetadata" {
					// fmt.Print("\n ResourceID being considered is ",childrenObject.Name)
					resourceObjectId := childrenObject.Value.ObjectID
					resourceType,AwsCdkPath := getResourceTypeAndPath(resourceObjectId, ctx)
					if resourceIdToType == nil {
						resourceIdToType = make(map[string]string)
					}
					if resourceIdToPath == nil {
						resourceIdToPath = make(map[string]string)
					}
					resourceIdToPath[childrenObject.Name] = AwsCdkPath
					resourceIdToType[childrenObject.Name] = resourceType
					resourceIds = append(resourceIds, childrenObject.Name)
				}
			}

		} else if nodeObject.Name == "id" {
			idIndex = index
		}
	}
	if stackIdToResourceIds == nil {
		stackIdToResourceIds = make(map[string][]string)
	}
	if stackToResourceToPath == nil {
		stackToResourceToPath = make(map[string] map[string] string)
	}	
	stackToResourceToPath[nodeObjects.Result[idIndex].Value.String()] = resourceIdToPath
	stackIdToResourceIds[nodeObjects.Result[idIndex].Value.String()] = resourceIds
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
			if stackToLogicalIds == nil {
				stackToLogicalIds = make(map[string][]string)
			}
			
			stackName = stackDataObject.Value.String()
		}
	}
	stackToLogicalIds[stackName] = logicalIds
}

func parseResourceDataBreakpointData(ctx context.Context , conn *rpcc.Conn) error {
	defer wg.Done()
	// getting current callstack and vars from the debugger process
	ev, err := pausedClient.Recv()
	if err != nil {
		return err
	}

	// extracting 
	//1. StackId-ConstructIDs-CDKPath
	//2. StackId-LogicalIDs
	// var stackIds []string
	if ev.Reason == "other"{
		for _, callFrame := range ev.CallFrames{
			if callFrame.FunctionName == "Construct"{
				for _,scopeChain := range callFrame.ScopeChain{
					if scopeChain.Type == "local"{
						localScope,_ := client.Runtime.GetProperties(ctx, &runtime.GetPropertiesArgs{ObjectID: *scopeChain.Object.ObjectID})
						// For storing the index of Id and Scope in localScopeResult array
						idIndex:=-1
						scopeIndex :=-1  
						for index,localScopeData := range localScope.Result{
							if localScopeData.Name == "id" && localScopeData.Value.String() == "\"BootstrapVersion\""{
									idIndex = index
							}else if localScopeData.Name == "scope"{
									scopeIndex = index
							}
						}
						if idIndex>=0 && scopeIndex>=0 {
							ScopeDataObjectId := localScope.Result[scopeIndex].Value.ObjectID
							getStackData(ScopeDataObjectId,ctx)	
							fmt.Print("\n stackToLogicalIds ", stackToLogicalIds)
							fmt.Print("\n resourceToType ", resourceIdToType)
							fmt.Print("\n stackIdToresourceID ", stackIdToResourceIds)
							fmt.Print("\n StackID -> ResourceID -> Path  ", stackToResourceToPath)
						}
						// For ending the client 
						// 1. Get the list of all the stacks present 
							// 1.a. From the time when map for location for each resource is being created
							// 1.b  When id value is  'CDKMetadata' Keep collecting stackNames
							// CDKMetadata is called for each stack before calling BootstrapVersion for any stack 
						// 2. When we encounter the 'CheckBootstrapVersion' or Just after BootstrapVersion of last stack End
						// Also For gathering 	


						// Since the Path already has ConstructID 
						// We can match the LogicalID to its ConstructID
					}
				}
			}
		}
	}
	
	
	client.Debugger.Resume(ctx, &debugger.ResumeArgs{})
	wg.Add(1)
	go parseResourceDataBreakpointData(ctx,conn)
	return nil
}

func main() {
	err := run(30000 * time.Second)
	if err != nil {
		log.Fatal(err)
	}
	
}


