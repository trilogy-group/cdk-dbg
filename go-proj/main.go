package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-sourcemap/sourcemap"
	"github.com/mafredri/cdp"
	"github.com/mafredri/cdp/devtool"
	"github.com/mafredri/cdp/protocol/debugger"
	"github.com/mafredri/cdp/protocol/runtime"
	"github.com/mafredri/cdp/rpcc"
)

type ResourceLocation struct {
	filePath string
	lineNumber int
	colNumber int
}
var client *cdp.Client
var pausedClient debugger.PausedClient
var wg sync.WaitGroup
var parDir string

//var resourceIds []string
var constructId string

//var stackLocations []string
//var mainLocations []string
var resourceIdToLocation = make(map[string]ResourceLocation)
var objectCount int = 0


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
	parDir = filepath.Dir(curDir)
	urlRegex := "^.*" + parDir + "/my-project2/node_modules/constructs/lib/construct.js$"
	columnNumber := 8
	client.Debugger.SetBreakpointByURL(ctx, &debugger.SetBreakpointByURLArgs{
		URLRegex:     &urlRegex,
		LineNumber:   129,
		ColumnNumber: &columnNumber,
	})
	client.Debugger.SetBreakpointByURL(ctx, &debugger.SetBreakpointByURLArgs{
		URLRegex:     &urlRegex,
		LineNumber:   367,
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

	//getResourceToLocation()
	return nil
}

// func getResourceToLocation() {
// 	if len(resourceIds) > 0 && len(mainLocations) > 0 {
// 		resourceIdToLocation[resourceIds[0]] = mainLocations[0]
// 	}
// 	for idx, id := range resourceIds {
// 		if idx > 0 && idx < len(stackLocations){
// 			resourceIdToLocation[id] = stackLocations[idx]
// 		}
// 	}
// 	fmt.Println(resourceIdToLocation)
// }

func parseBreakpointData(ctx context.Context) error {
	defer wg.Done()
	ev, err := pausedClient.Recv()
	if err != nil {
		return err
	}
	fmt.Println(ev.Reason)

	for _, callFrame := range ev.CallFrames {
		fmt.Println(callFrame.URL)
		stackFile := "file://" + parDir + "/my-project2/lib/my-project2-stack.ts"
		mainFile := "file://" + parDir + "/my-project2/bin/my-project2.ts"
		var src *debugger.GetScriptSourceReply
		z, _ := client.Runtime.GetProperties(ctx, &runtime.GetPropertiesArgs{ObjectID: *callFrame.ScopeChain[0].Object.ObjectID})
		if callFrame.FunctionName == "Construct" && len(z.Result) > 1 {
			fmt.Println("ID")
			json.Unmarshal(z.Result[1].Value.Value, &constructId)
			// fmt.Println(constructId)
			// if len(constructId) > 0 && constructId != "Tree" {
			// 	resourceIds = append(resourceIds, constructId)
			// }
		}
		if callFrame.URL == stackFile || callFrame.URL == mainFile {
			scriptId := callFrame.Location.ScriptID
			src, err = client.Debugger.GetScriptSource(ctx, &debugger.GetScriptSourceArgs{ScriptID: scriptId})
			if err != nil {
				return err
			}

			mapURL := callFrame.URL
			splitSrcLines := strings.Split(src.ScriptSource, "\n")
			sourceMapBase64 := strings.Replace(splitSrcLines[len(splitSrcLines)-1], "//# sourceMappingURL=data:application/json;charset=utf-8;base64,", "", -1)

			sourceMap, err := base64.StdEncoding.DecodeString(sourceMapBase64)
			if err != nil {
				return err
			}
			smap, err := sourcemap.Parse(mapURL, sourceMap)
			if err != nil {
				panic(err)
			}

			// chrome devtools protocol and it's implementation cdp has both line and column as 0 indexed
			// For the sourcemap library used is considering line number as 1 indexed and column number as 0 indexed (both input and output)
			// although their doc mentions that both line and column are 0 indexed
			// doc: https://docs.google.com/document/d/1U1RGAehQwRypUTovF1KRlpiOFze0b-_2gc6fAH0KY0k/edit
			genline, gencol := callFrame.Location.LineNumber+1, *callFrame.Location.ColumnNumber
			fmt.Println("Variables of interest")
			file, fn, sourceline, sourcecol, ok := smap.Source(genline, gencol)
			fmt.Println(file, fn, sourceline, sourcecol, ok)
			_, isPresent := resourceIdToLocation[constructId]
			if len(constructId) > 0 && constructId != "Tree" && !isPresent {
				// var x = ResourceLocation{file, sourceline, sourcecol + 1}
				if callFrame.URL == mainFile && objectCount == 0 {
					//mainLocations = append(mainLocations, file + " " + fmt.Sprint(sourceline) + ":" + fmt.Sprint(sourcecol + 1))
					resourceIdToLocation[constructId] = file + " " + fmt.Sprint(sourceline) + ":" + fmt.Sprint(sourcecol+1)
					objectCount++
				} else if callFrame.URL == stackFile && objectCount > 0 {
					//stackLocations = append(stackLocations, file + " " + fmt.Sprint(sourceline) + ":" + fmt.Sprint(sourcecol + 1))
					resourceIdToLocation[constructId] = file + " " + fmt.Sprint(sourceline) + ":" + fmt.Sprint(sourcecol+1)
					objectCount++
				}
			}

			// original use's source code
			cnt := smap.SourceContent(strings.Replace(mapURL, "file://", "", -1))
			fmt.Println(cnt)
		}
		// for viewing variables
		var a []*runtime.GetPropertiesReply

		for _, scope := range callFrame.ScopeChain {

			// for viewing variables
			z, err := client.Runtime.GetProperties(ctx, &runtime.GetPropertiesArgs{ObjectID: *scope.Object.ObjectID})
			if err != nil {
				fmt.Println("Error: ", err)
			}
			a = append(a, z)
		}

		// breakpoint here for checking a and b
		fmt.Sprintf("To enable Breakpoint at this line for a and b")
	}
	client.Debugger.Resume(ctx, &debugger.ResumeArgs{})
	wg.Add(1)
	go parseBreakpointData(ctx)
	fmt.Println(resourceIdToLocation)
	return nil
}

func main() {
	err := run(300 * time.Second)
	if err != nil {
		log.Fatal(err)
	}
}
