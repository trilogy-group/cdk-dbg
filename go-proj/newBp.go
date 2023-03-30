package main

// to use this go file commend out the main.go file 
// and make the kmain at 409 line to main


import (
	"context"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"

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
	"github.com/trilogy-group/cloudfix-linter-developer/logger"
)

var client *cdp.Client
var pausedClient debugger.PausedClient
var wg sync.WaitGroup
var stackIdToResourceIds map[string][]string // stackID to resourcesID
var stackToLogicalIds map[string][]string    // stackID to logicalIds
var resourceIdToType map[string]string
var stackToResourceToPath map[string]map[string]string
var resourceIdToPath map[string]string
var stackIds []string

func run(timeout time.Duration) error {
	// ctx, cancel := context.WithTimeout(context.Background(), timeout)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Use the DevTools HTTP/JSON API to manage targets (e.g. pages, webworkers).
	devt := devtool.New("http://localhost:2333")
	pt, err := devt.Get(ctx, devtool.Node)
	if err != nil {
		return err
	}

	fmt.Println("KK", pt.WebSocketDebuggerURL)
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
		fmt.Print("\n Error while enbling debugger ", err)
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

	urlRegex := "^.*" + parDir + "/my-project2/node_modules/aws-cdk-lib/core/lib/stack-synthesizers/_shared.js$"
	columnNumber := 921
	client.Debugger.SetBreakpointByURL(ctx, &debugger.SetBreakpointByURLArgs{
		URLRegex:     &urlRegex,
		LineNumber:   0,
		ColumnNumber: &columnNumber,
	})
	urlRegex2 := "^.*" + parDir + "/my-project2/node_modules/constructs/lib/construct.js$"
	columnNumber2 := 9
	client.Debugger.SetBreakpointByURL(ctx, &debugger.SetBreakpointByURLArgs{
		URLRegex:     &urlRegex2,
		LineNumber:   368,
		ColumnNumber: &columnNumber2,
	})
	pausedClient, err = client.Debugger.Paused(ctx)
	client.Debugger.Resume(ctx, &debugger.ResumeArgs{})

	//used to continue program execution if the program is currently paused waiting for a debugger to attach.
	//If the program is not currently waiting for a debugger, this method will simply return immediately without doing anything.
	err = client.Runtime.RunIfWaitingForDebugger(ctx)
	if err != nil {
		return err
	}

	// wg.Add(1)
	parseResourceDataBreakpointData(ctx, conn)

	// wg.Wait()

	return nil
}

// var defaultVar :=["constructor"]
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

func GetCdkPath(CfnOptionsObjectId *runtime.RemoteObjectID, ctx context.Context) string {
	cfnOptionsData, errCfn := client.Runtime.GetProperties(ctx, &runtime.GetPropertiesArgs{ObjectID: *CfnOptionsObjectId})
	if errCfn != nil {
		fmt.Print("\n Not able to get CfnOptions from callstackData")
	}
	for _, insideCfnOptionData := range cfnOptionsData.Result {
		if insideCfnOptionData.Name == "metadata" {
			metaData, _ := client.Runtime.GetProperties(ctx, &runtime.GetPropertiesArgs{ObjectID: *insideCfnOptionData.Value.ObjectID})
			for _, insideMetaData := range metaData.Result {
				if insideMetaData.Name == "aws:cdk:path" {
					return insideMetaData.Value.String()
				}
			}
		}
	}
	return ""
}

func getResourceTypeAndPath(dataObj *runtime.RemoteObjectID, ctx context.Context) (string, string) {
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
					AwsCdkPath = GetCdkPath(cfnResourceObject.Value.ObjectID, ctx)
					//its inside cfnOptions -> metadata -> aws:cdk:path
				}
			}
			return resourceType, AwsCdkPath
		}
	}
	return "", ""
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
					resourceType, AwsCdkPath := getResourceTypeAndPath(resourceObjectId, ctx)
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
		stackToResourceToPath = make(map[string]map[string]string)
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

func getStackId(dataObj *runtime.RemoteObjectID, ctx context.Context) (string, error) {
	stackDataObjects, errStackId := client.Runtime.GetProperties(ctx, &runtime.GetPropertiesArgs{ObjectID: *dataObj})
	var stackId string
	if errStackId != nil {
		logger.DevLogger().Error("Could not get scope object while getting stackIds in debuggerClient. Error:", errStackId)
		return "", errors.New("CANNOTGETSTACKIDS")
	}
	for _, stackDataObjects := range stackDataObjects.Result {
		if stackDataObjects.Name == "_stackName" {
			stackId = stackDataObjects.Value.String()
		}
	}
	return stackId, nil
}

func CheckToEndConnection(bootStrapIds []string, stackIds []string) bool {
	if len(bootStrapIds) == len(stackIds) {
		sort.Strings(bootStrapIds)
		sort.Strings(stackIds)
		// Compare the slices element by element
		equal := true
		for i, v := range bootStrapIds {
			if v != stackIds[i] {
				equal = false
				break
			}
		}
		return equal
	}
	return false
}

func getDataFromNBP(pathObjectId *runtime.RemoteObjectID, ctx context.Context) (string, error) {
	insidePathData, errPD := client.Runtime.GetProperties(ctx, &runtime.GetPropertiesArgs{ObjectID: *pathObjectId})
	if errPD != nil {
		logger.DevLogger().Error("Error occured while getting Path Object, inside the meta. Error:", errPD)
		return "", nil
	}
	for _, insidePath := range insidePathData.Result {
		if insidePath.Name == "0" {
			zerothObject, errZ := client.Runtime.GetProperties(ctx, &runtime.GetPropertiesArgs{ObjectID: *insidePath.Value.ObjectID})
			if errZ != nil {
				logger.DevLogger().Error("Error occured while Oth Object, inside PathObject. Error:", errZ)
				return "", nil
			}
			for _, insideZerothObject := range zerothObject.Result {
				if insideZerothObject.Name == "data" {
					fmt.Print("\n insideZerothObject ", insideZerothObject.Name)
					return insideZerothObject.Value.String(), nil
				}
			}

		}
	}
	return "", errors.New("DATANOTFOUND")
}

func getMetaData(metaDataObjectId *runtime.RemoteObjectID, ctx context.Context) error {
	metaDataObjects, errMd := client.Runtime.GetProperties(ctx, &runtime.GetPropertiesArgs{ObjectID: *metaDataObjectId})
	var pathToId map[string]string
	if errMd != nil {
		logger.DevLogger().Error("Error while getting metaData Object in DebuggerClient. Error:", errMd)
	}
	for _, insideMeta := range metaDataObjects.Result {
		if *insideMeta.IsOwn {
			if pathToId == nil {
				pathToId = make(map[string]string)
			}
			PData, err := getDataFromNBP(insideMeta.Value.ObjectID, ctx)
			if err != nil {
				if err.Error() == "DATANOTFOUND" {
					logger.DevLogger().Error("Error while getting data from inside the path (meta->path->error).Could not find data inside(meta-", insideMeta.Name, "-")
					return err
				} else {
					logger.DevLogger().Error("Error while getting data from inside the path (meta->path->error). Error:", err)
					return err
				}

			}
			pathToId[insideMeta.Name] = PData
		}

	}
	fmt.Print(pathToId)
	return nil
}

func CheckBreakPointFile(hitBreakPointsObject []string, ctx context.Context) (string, error) {
	fmt.Print(hitBreakPointsObject)
	for _, breakPoint := range hitBreakPointsObject {
		if strings.Contains(breakPoint, "node_modules/constructs/lib/construct.js") {
			return "construct", nil
		} else if strings.Contains(breakPoint, "node_modules/aws-cdk-lib/core/lib/stack-synthesizers/_shared.js") {
			return "_shared", nil
		} else {
			return breakPoint, errors.New("UNIDENTIFIEDFILE")
		}

	}
	return "", errors.New("UNIDENTIFIEDFILE")
}
func parseResourceDataBreakpointData(ctx context.Context, conn *rpcc.Conn) error {
	// defer wg.Done()
	// getting current callstack and vars from the debugger process
	ev, err := pausedClient.Recv()
	if err != nil {
		return err
	}
	if len(ev.HitBreakpoints) > 0 { // don't check for break on start
		fileName, err := CheckBreakPointFile(ev.HitBreakpoints, ctx)
		if err != nil {
			if err.Error() == "UNIDENTIFIEDFILE" {
				logger.DevLogger().Error("Break point put onUnidentified file. BreakPoint:", fileName, " Error:", err)
				return err
			}
		}
		if fileName == "Construct" {

		} else if fileName == "_shared" {
			if ev.Reason == "other" {
				for _, callFrame := range ev.CallFrames {
					if callFrame.FunctionName == "addStackArtifactToAssembly" {
						for _, scopeChain := range callFrame.ScopeChain {
							if scopeChain.Type == "local" {
								localScope, errLs := client.Runtime.GetProperties(ctx, &runtime.GetPropertiesArgs{ObjectID: *scopeChain.Object.ObjectID})
								if errLs != nil {
									logger.DevLogger().Error("Error while getting the localScope data in debugger client. Error:", errLs)
								}
								for _, localScopeData := range localScope.Result {
									// stackIndex := -1
									// metaIndex := -1
									if localScopeData.Name == "stack" {
										getStackData(localScopeData.Value.ObjectID, ctx)
										// Here get the stackData as previously done
									} else if localScopeData.Name == "meta" {
										// fmt.Print(localScopeData)
										errMd := getMetaData(localScopeData.Value.ObjectID, ctx)
										fmt.Print(errMd)
										conn.Close()
									}

								}
							}
						}
					}
				}
			}
			fmt.Print(stackToResourceToPath,stackToLogicalIds)
		}
	}

	// extracting
	//1. StackId-ConstructIDs-CDKPath
	//2. StackId-LogicalIDs
	// var stackIds []string

	client.Debugger.Resume(ctx, &debugger.ResumeArgs{})
	// wg.Add(1)
	parseResourceDataBreakpointData(ctx, conn)
	return nil
}



func kmain() {
	err := run(30000 * time.Second)
	if err != nil {
		log.Fatal(err)
	}

}

// if ev.Reason == "other"{
// 	for _, callFrame := range ev.CallFrames{
// 		if callFrame.FunctionName == "Construct"{
// 			for _,scopeChain := range callFrame.ScopeChain{
// 				if scopeChain.Type == "local"{
// 					localScope,_ := client.Runtime.GetProperties(ctx, &runtime.GetPropertiesArgs{ObjectID: *scopeChain.Object.ObjectID})
// 					// For storing the index of Id and Scope in localScopeResult array
// 					bootStrapVersionIdIndex:=-1
// 					CdkMetaDataIndex:=-1
// 					scopeIndex :=-1
// 					for index,localScopeData := range localScope.Result{
// 						if localScopeData.Name == "id" && localScopeData.Value.String() == "\"BootstrapVersion\""{
// 								bootStrapVersionIdIndex = index
// 						}else if localScopeData.Name == "id" && localScopeData.Value.String() == "\"CDKMetadata\"" {
// 								CdkMetaDataIndex = index
// 						}else if localScopeData.Name == "scope"{
// 								scopeIndex = index
// 						}
// 					}
// 					//This gets called before BootstrapVersion is called for any stack
// 					if CdkMetaDataIndex>=0 && scopeIndex>=0{
// 						ScopeDataObjectId := localScope.Result[scopeIndex].Value.ObjectID
// 						stackID,err :=getStackId(ScopeDataObjectId,ctx)
// 						if err!= nil{
// 							return err
// 						}
// 						stackIds = append(stackIds, stackID)
// 					}
// 					// By this time all the stackIds have been prepared
// 					if bootStrapVersionIdIndex>=0 && scopeIndex>=0 {
// 						ScopeDataObjectId := localScope.Result[scopeIndex].Value.ObjectID
// 						var allStackIds []string
// 						stackId,err:=getStackId(ScopeDataObjectId,ctx)
// 						if err!= nil{
// 							logger.DevLogger().Error("Error while fetching stackID for BootStrapVersionID")
// 							return err
// 						}
// 						allStackIds = append(allStackIds, stackId)
// 						getStackData(ScopeDataObjectId,ctx)
// 						fmt.Print("\n stackToLogicalIds ", stackToLogicalIds)
// 						fmt.Print("\n resourceToType ", resourceIdToType)
// 						fmt.Print("\n stackIdToresourceID ", stackIdToResourceIds)
// 						fmt.Print("\n StackID -> ResourceID -> Path  ", stackToResourceToPath)
// 						if CheckToEndConnection(allStackIds,stackIds){
// 							conn.Close()
// 						}

// 					}
// 					// For ending the client
// 					// 1. Get the list of all the stacks present
// 						// 1.a. From the time when map for location for each resource is being created
// 						// 1.b  When id value is  'CDKMetadata' Keep collecting stackNames
// 						// CDKMetadata is called for each stack before calling BootstrapVersion for any stack
// 					// 2. When we encounter the 'CheckBootstrapVersion' or Just after BootstrapVersion of last stack End
// 					// Also For gathering

// 					// Since the Path already has ConstructID
// 					// We can match the LogicalID to its ConstructID
// 				}
// 			}
// 		}
// 	}
// }
