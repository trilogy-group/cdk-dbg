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

	for _, callFrame := range ev.CallFrames {
		fmt.Println(callFrame.URL)
		// for viewing variables
		var a []*runtime.GetPropertiesReply
		var b []*runtime.AwaitPromiseReply

		for _, scope := range callFrame.ScopeChain {

			// for viewing variables
			z, err := client.Runtime.GetProperties(ctx, &runtime.GetPropertiesArgs{ObjectID: *scope.Object.ObjectID})
			if err != nil {
				fmt.Println("Error: ", err)
			}
			a = append(a, z)

			y, err := client.Runtime.AwaitPromise(ctx, &runtime.AwaitPromiseArgs{PromiseObjectID: *callFrame.ScopeChain[0].Object.ObjectID})
			if err != nil {
				fmt.Println("Error: ", err)
			}
			b = append(b, y)
		}

		// breakpoint here for checking a and b
		fmt.Sprintf("To enable Breakpoint at this line for a and b")
	}
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
