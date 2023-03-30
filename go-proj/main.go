package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"

	// "log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/go-sourcemap/sourcemap"
	"github.com/mafredri/cdp"
	"github.com/mafredri/cdp/devtool"
	"github.com/mafredri/cdp/protocol/debugger"
	"github.com/mafredri/cdp/protocol/runtime"
	"github.com/mafredri/cdp/rpcc"
	"github.com/trilogy-group/cloudfix-linter-developer/logger"
)

type ResourceLocation struct {
	filePath   string
	lineNumber int
	colNumber  int
}
var stackFile = "/lib/"
var mainFile = "/bin/"

var client *cdp.Client
var pausedClient debugger.PausedClient
var wg sync.WaitGroup
var stackIdToResourceIds map[string][]string // stackID to resourcesID
var stackToLogicalIds map[string][]string    // stackID to logicalIds
var resourceIdToType map[string]string
var stackToResourceToPath map[string]map[string]string
var resourceIdToPath map[string]string
var stackIds []string
var parDir string
var pathToLocation map[string][]ResourceLocation

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
	parDir = filepath.Dir(curDir)

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
	columnNumber := 17
	client.Debugger.SetBreakpointByURL(ctx, &debugger.SetBreakpointByURLArgs{
		URLRegex:     &urlRegex,
		LineNumber:   323,
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

	// wg.Add(1)
	parseResourceDataBreakpointData(ctx, conn)

	// wg.Wait()

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

func getInternalProperties(object *runtime.GetPropertiesReply, ctx context.Context) {
	internalProp := object.InternalProperties
	for _, internalPropData := range internalProp {
		Data, _ := client.Runtime.GetProperties(ctx, &runtime.GetPropertiesArgs{ObjectID: *internalPropData.Value.ObjectID})
		fmt.Print(Data)

		// for _,insideData := range Data.Result{
		// 	fmt.Print(insideData)
		// }
	}
}
func getPath(callFrame debugger.CallFrame, ctx context.Context) (string, error) {
	result, err := client.Debugger.EvaluateOnCallFrame(ctx, &debugger.EvaluateOnCallFrameArgs{
		CallFrameID: callFrame.CallFrameID,
		Expression:  "this.path",
	})
	if err != nil {
		logger.DevLogger().Error("Error while Evaluating 'this.path' in debugger.client. Error:", err)
		return "", err
	}
	var path string
	json.Unmarshal(result.Result.Value, &path)
	fmt.Println("This path is ", path)
	return path, nil

}

func getFileLocation(callFrame debugger.CallFrame, ctx context.Context, path string) (ResourceLocation, error) {
	var fileLocation ResourceLocation
	scriptId := callFrame.Location.ScriptID
	src, err := client.Debugger.GetScriptSource(ctx, &debugger.GetScriptSourceArgs{ScriptID: scriptId})
	if err != nil {
		return fileLocation, err
	}
	mapURL := callFrame.URL
	splitSrcLines := strings.Split(src.ScriptSource, "\n")
	sourceMapBase64 := strings.Replace(splitSrcLines[len(splitSrcLines)-1], "//# sourceMappingURL=data:application/json;charset=utf-8;base64,", "", -1)

	sourceMap, err := base64.StdEncoding.DecodeString(sourceMapBase64)
	if err != nil {
		return fileLocation, err
	}
	smap, err := sourcemap.Parse(mapURL, sourceMap)
	if err != nil {
		panic(err)
		return fileLocation, err
	}

	// chrome devtools protocol and it's implementation cdp has both line and column as 0 indexed
	// For the sourcemap library used is considering line number as 1 indexed and column number as 0 indexed (both input and output)
	// although their doc mentions that both line and column are 0 indexed
	// doc: https://docs.google.com/document/d/1U1RGAehQwRypUTovF1KRlpiOFze0b-_2gc6fAH0KY0k/edit
	genline, gencol := callFrame.Location.LineNumber+1, *callFrame.Location.ColumnNumber
	file, _, sourceline, sourcecol, _ := smap.Source(genline, gencol)
	// fmt.Println(file, fn, sourceline, sourcecol, ok)
	// fmt.Print(isPresent)
	// fmt.Print("FOR Path ",path," mainfile Location :",sourceline,sourcecol,file)
	// _, MappingPresent := pathToLocation[path]
	// if !MappingPresent {
	// 	ArrayResourceLocation := make([]ResourceLocation, 1)
	// 	mainFileLocation := ResourceLocation{lineNumber: sourceline, colNumber: sourcecol, filePath: file}
	// 	ArrayResourceLocation = append(ArrayResourceLocation, mainFileLocation)
	// 	pathToLocation[path] = append(pathToLocation[path], mainFileLocation)
	// }
	fileLocation.colNumber = sourcecol
	fileLocation.lineNumber = sourceline
	fileLocation.filePath = file

	return fileLocation, nil
}
func parseResourceDataBreakpointData(ctx context.Context, conn *rpcc.Conn) error {
	// defer wg.Done()
	// getting current callstack and vars from the debugger process
	ev, err := pausedClient.Recv()
	if err != nil {
		return err
	}

	// extracting
	//1. StackId-ConstructIDs-CDKPath
	//2. StackId-LogicalIDs
	// var stackIds []string
	addChildIndex := -1
	mainFileIndex := -1
	stackFileIndex := -1
	// var src *debugger.GetScriptSourceReply
	if ev.Reason == "other" {
		for index, callFrame := range ev.CallFrames {
			if callFrame.FunctionName == "Construct" {
				for _, scopeChain := range callFrame.ScopeChain {
					if scopeChain.Type == "local" {
						localScope, _ := client.Runtime.GetProperties(ctx, &runtime.GetPropertiesArgs{ObjectID: *scopeChain.Object.ObjectID})
						// For storing the index of Id and Scope in localScopeResult array
						bootStrapVersionIdIndex := -1
						CdkMetaDataIndex := -1
						scopeIndex := -1
						for index, localScopeData := range localScope.Result {
							if localScopeData.Name == "id" && localScopeData.Value.String() == "\"BootstrapVersion\"" {
								bootStrapVersionIdIndex = index
							} else if localScopeData.Name == "id" && localScopeData.Value.String() == "\"CDKMetadata\"" {
								CdkMetaDataIndex = index
							} else if localScopeData.Name == "scope" {
								scopeIndex = index
							}
						}
						//This gets called before BootstrapVersion is called for any stack
						if CdkMetaDataIndex >= 0 && scopeIndex >= 0 {
							ScopeDataObjectId := localScope.Result[scopeIndex].Value.ObjectID
							stackID, err := getStackId(ScopeDataObjectId, ctx)
							if err != nil {
								return err
							}
							stackIds = append(stackIds, stackID)
						}
						// By this time all the stackIds have been prepared
						if bootStrapVersionIdIndex >= 0 && scopeIndex >= 0 {
							ScopeDataObjectId := localScope.Result[scopeIndex].Value.ObjectID
							var allStackIds []string
							stackId, err := getStackId(ScopeDataObjectId, ctx)
							if err != nil {
								logger.DevLogger().Error("Error while fetching stackID for BootStrapVersionID")
								return err
							}
							allStackIds = append(allStackIds, stackId)
							getStackData(ScopeDataObjectId, ctx)
							fmt.Print("\n stackToLogicalIds ", stackToLogicalIds)
							fmt.Print("\n resourceToType ", resourceIdToType)
							fmt.Print("\n stackIdToresourceID ", stackIdToResourceIds)
							fmt.Print("\n StackID -> ResourceID -> Path  ", stackToResourceToPath)
							// if CheckToEndConnection(allStackIds,stackId){
							// 	conn.Close()
							// }

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
			}else if callFrame.FunctionName == "addChild" {
				addChildIndex = index
			}
			if !strings.Contains(callFrame.URL, "node_modules") {
				if strings.Contains(callFrame.URL, "bin"){ // think about the cases when multiple stacks present
					// MainFile will be come first when only App is being constructed
					// otherwise stackFile comes first
					mainFileIndex = index
					if stackFile == ""{
						fmt.Print(callFrame.URL)
						mainFile = callFrame.URL
						strParts := strings.Split(mainFile,"bin")
						stackFile = strParts[0]
					}
				}else if strings.Contains(callFrame.URL, stackFile){
					stackFileIndex = index
				}
			}

		}
		var path string
		if addChildIndex >= 0 && (mainFileIndex >= 0 || stackFileIndex >= 0) {
			// create a map of path to location
			path, err = getPath(ev.CallFrames[addChildIndex], ctx)
			if err != nil {
				return err
			}
			if pathToLocation == nil {
				pathToLocation = make(map[string][]ResourceLocation)
				// pathToLocation[path] = ResourceLocation{colNumber: 0, lineNumber: 0, filePath: ""}]
			}
			// pathToLocation[path] = ResourceLocation{colNumber: 0, lineNumber: 0, filePath: ""}
			if mainFileIndex >= 0{
				callFrame := ev.CallFrames[mainFileIndex]
				mainFileLocation, err := getFileLocation(callFrame, ctx, path)
				if err != nil {
					logger.DevLogger().Error("Could not get mainfile location for path ", path, " Error:", err)
				}
				pathToLocation[path] = append(pathToLocation[path], mainFileLocation)

			}
			if stackFileIndex >= 0{
				callFrame := ev.CallFrames[stackFileIndex]
				stackFileLocation, err := getFileLocation(callFrame, ctx, path)
				if err != nil {
					logger.DevLogger().Error("Could not get stack location for path ", path, " Error:", err)
				}
				pathToLocation[path] = append(pathToLocation[path], stackFileLocation)
			}
		}

		fmt.Print("MAPPINGS for path ", path, " are ", pathToLocation[path])
	}

	client.Debugger.Resume(ctx, &debugger.ResumeArgs{})
	// wg.Add(1)
	parseResourceDataBreakpointData(ctx, conn)
	return nil
}

func main() {
	err := run(30000 * time.Second)
	if err != nil {
		log.Fatal(err)
	}

}
