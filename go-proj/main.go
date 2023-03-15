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
	urlRegex := "^.*" + parDir + "/my-project2/node_modules/constructs/lib/construct.js$"
	columnNumber := 9
	client.Debugger.SetBreakpointByURL(ctx, &debugger.SetBreakpointByURLArgs{
		URLRegex:     &urlRegex,
		LineNumber:   368,
		ColumnNumber: &columnNumber,
	})

	pausedClient, err = client.Debugger.Paused(ctx)
	client.Debugger.Resume(ctx, &debugger.ResumeArgs{})
	err = client.Runtime.RunIfWaitingForDebugger(ctx)

	if err != nil {
		return err
	}

	wg.Add(1)
	go parseBreakpointData(ctx)

	wg.Wait()
	return nil
}

func parseBreakpointData(ctx context.Context) error {
	defer wg.Done()
	ev, err := pausedClient.Recv()
	if err != nil {
		return err
	}
	fmt.Println(ev.Reason)
	var Id_Scope map[string][]string

	var functionNames = map[string]string{
		"Construct":"Construct",
	}
	for indexCF, callFrame := range ev.CallFrames {
		fmt.Println(callFrame.URL)
		val,ok := functionNames[callFrame.FunctionName]

		// To see what data is being retuned in z visit 
		//https://pkg.go.dev/github.com/mafredri/cdp@v0.33.0/protocol/runtime#PropertyDescriptor
		//https://pkg.go.dev/github.com/mafredri/cdp@v0.33.0/protocol/runtime#GetPropertiesReply
		if ok && ev.Reason !="Break on start"{
			fmt.Print("FUNCTION NAME FOUND ",val," INDEX OF CF ",indexCF)
			z, _ := client.Runtime.GetProperties(ctx, &runtime.GetPropertiesArgs{ObjectID: *callFrame.ScopeChain[0].Object.ObjectID})
			for ind,data := range z.Result{
				fmt.Print("\nDATA FOR INDEX ",ind)
				fmt.Print("\nDATA = ",data)
				fmt.Print("\n Data.Value =",data.Value)
				if data.Value.ClassName !=nil{ 
					fmt.Print("\n Data.Value.Class ",*data.Value.ClassName)
					_,ok := Id_Scope[data.Name]
				if ok{
					var res []string 
					res = append(res, *data.Value.Description)
					Id_Scope[data.Name] = res
				}else{
					Id_Scope[data.Name] = append(Id_Scope[data.Name], *data.Value.Description)
				}
				}
				if data.Value.Description != nil{
					fmt.Print("\n Data.Value.Description ",*data.Value.Description)
				}
				fmt.Print("\n Data.Value.Type ",data.Value.Type)
				fmt.Print("\n Data.Value.Preview ",data.Value.Preview)
				fmt.Print("\n Data.Value.CustomPreview ",data.Value.CustomPreview)
				
					


				

				// scope,ok := Id_Scope[data.Name]
				// if ok{
				// 	var val []string 
				// 	val = append(val, string(*data.Value.ClassName))
				// 	Id_Scope[data.Name] = val
				// }
				// fmt.Print("\nWritable ",*data.Writable)
				// fmt.Print("\nISOWN ",*data.IsOwn)
			}
		}
		fmt.Print("SCOPE RESOURCE MAP ", Id_Scope)
		
	}
	fmt.Println("\nONE DONE \n")
	client.Debugger.Resume(ctx, &debugger.ResumeArgs{})
	wg.Add(1)
	go parseBreakpointData(ctx)
	return nil
}

func main() {
	err := run(300 * time.Second)
	if err != nil {
		log.Fatal(err)
	}
}
