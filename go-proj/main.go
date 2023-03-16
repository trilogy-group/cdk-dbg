package main

import (
	"context"
	"fmt"
	"log"
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
	//"/my-project2/node_modules/ts-node/dist/bin.js$"
	// urlRegex := "^.*" + parDir + "/my-project2/node_modules/constructs/lib/construct.js$"
	// columnNumber :=0
	// client.Debugger.SetBreakpointByURL(ctx, &debugger.SetBreakpointByURLArgs{
	// 	URLRegex:     &urlRegex,
	// 	LineNumber:   131,
	// 	ColumnNumber: &columnNumber,
	// })
	urlRegex := "^.*" + parDir + "/my-project2/node_modules/constructs/lib/construct.js$"
	columnNumber :=8
	client.Debugger.SetBreakpointByURL(ctx, &debugger.SetBreakpointByURLArgs{
		URLRegex:     &urlRegex,
		LineNumber:   129,
		ColumnNumber: &columnNumber,
	})

	pausedClient, err = client.Debugger.Paused(ctx)
	client.Debugger.Resume(ctx, &debugger.ResumeArgs{})
	err = client.Runtime.RunIfWaitingForDebugger(ctx)

	if err != nil {
		return err
	}

	wg.Add(1)
	go parseResourceDataBreakpointData(ctx)

	wg.Wait()
	return nil
}

func parseResourceDataBreakpointData(ctx context.Context) error {
	defer wg.Done()
	ev, err := pausedClient.Recv()
	if err != nil {
		return err
	}
	stackLogicalId := "MyStack"
	var unnecessary_IDs = map[string]string {
		"constructor":"constructor",
		"__defineGetter__":"__defineGetter__",
		"__defineSetter__":"__defineSetter__",
		"hasOwnProperty":"hasOwnProperty",
		"__lookupGetter__":"__lookupGetter__",
		"__lookupSetter__":"__lookupSetter__",
		"isPrototypeOf":"isPrototypeOf",
		"propertyIsEnumerable":"propertyIsEnumerable",
		"toString":"toString",
		"valueOf":"valueOf",
		"__proto__":"__proto__",
	}
	fmt.Println(ev.Reason)
	var logicalIds map[string]string 
	if ev.Reason == "other"{
		for _,callFrame := range ev.CallFrames{
			if callFrame.FunctionName == "synth"{
				fmt.Print("Functionname ",callFrame.FunctionName)
				if &callFrame.This.Description != nil {
					fmt.Print("\nTHIS ID ,",*callFrame.This.Description)
				}

				//accessing "this" object/instance  of the callStack
				appData, err := client.Runtime.GetProperties(ctx, &runtime.GetPropertiesArgs{ObjectID: *callFrame.This.ObjectID})
				if err != nil {
					fmt.Print("Error in getting callstack detials")
				}
				// iterating over all the objects present in the this object
				for _, data := range appData.Result{
					// if &data.Value != nil && data.Value != nil &&  data.Value.ClassName != nil{
						if data.Name == "node" {
							fmt.Print("\nDATA INSIDE NODE ")
							nodeData,err := client.Runtime.GetProperties(ctx, &runtime.GetPropertiesArgs{ObjectID: *data.Value.ObjectID})
							if err != nil {
								fmt.Print("ERROR in 1 level deep going")
							}
							fmt.Print("\n getting data inside node ")
							// Iterating all the objects inside node
							for _,insideNode := range nodeData.Result{
								// if &insideNode.Value != nil && insideNode.Value != nil &&  insideNode.Value.ClassName != nil{
									if insideNode.Name == "_children"{
										fmt.Print("\n Found children inside node ...\n")
										childrenData,err := client.Runtime.GetProperties(ctx, &runtime.GetPropertiesArgs{ObjectID: *insideNode.Value.ObjectID})
										if err != nil{
											fmt.Print("Error in getting children object")
										}
										for _,insideChildren := range childrenData.Result{
											if insideChildren.Name == stackLogicalId{
												fmt.Print("\n FOUND MYSTACK ",insideChildren.Value)
												stackData,err := client.Runtime.GetProperties(ctx, &runtime.GetPropertiesArgs{ObjectID: *insideChildren.Value.ObjectID})
												if err != nil {
													fmt.Print("Error in Getting stackObject inside children ... ")
												}
												// fmt.Print("\n ENTIRE STACK DATA ",stackData)
												for _,insideStack := range stackData.Result{
													fmt.Print("\n Inside stack now ...")
													if insideStack.Name == "_logicalIds"{
														fmt.Print("\n found logicalID for ",insideChildren.Name)
														// fmt.Print("\n PRINTING insideStack NOw",insideStack)
														logicalIdsObject,err := client.Runtime.GetProperties(ctx, &runtime.GetPropertiesArgs{ObjectID: *insideStack.Value.ObjectID})
														if err != nil{
															fmt.Print("\n Error in getting logicalIDs object")
											
														}
														// fmt.Print("\n  logical ID object  ",logicalIdsObject.Result)
														for _,logicalIdData := range logicalIdsObject.Result{
															if logicalIdData.Name == "reverse"{
																insideLogicalId,_ :=  client.Runtime.GetProperties(ctx, &runtime.GetPropertiesArgs{ObjectID: *logicalIdData.Value.ObjectID})
																
																fmt.Print("\n Inside logicalID object ")

																if len(insideLogicalId.Result)>=11{
																	for lIdIndex,lId := range insideLogicalId.Result{
																		fmt.Print("\n entire object at index ",lIdIndex," \narray of LIds ",lId.Name)
																		_,ok := unnecessary_IDs[lId.Name]

																		if !ok{
																			fmt.Print("\n found new logicalID writing it in map of logicalID")
																			// logicalIds[lId.Name]= lId.Name
																			fmt.Print("\n logical ID var is = ",logicalIds[lId.Name])
																		}
																	} 
																	

																}
																// client.Debugger.Disable(ctx)
																
																// answer := logicalIdData
																// fmt.Print("\nFinal answer ",answer,"\n Value of ",answer.Value)
																// logicalIds = append(logicalIds, answer.Name)
															}
														}
													}
													
												}

											}
										}
										
									}
								// }
							}
						}
					// } 
				}
				
			}
		}
	}
	// fmt.Print("\n var logical ID ",logicalIds)
	client.Debugger.Resume(ctx, &debugger.ResumeArgs{})
	wg.Add(1)
	go parseResourceDataBreakpointData(ctx)
	return nil
}

func main() {
	err := run(30000 * time.Second)
	if err != nil {
		log.Fatal(err)
	}
}
