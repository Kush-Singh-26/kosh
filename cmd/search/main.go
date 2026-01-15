//go:build js && wasm
// +build js,wasm

package main

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"syscall/js"

	"my-ssg/builder/models"
	"my-ssg/builder/search"
)

var index models.SearchIndex

func main() {
	c := make(chan struct{}, 0)
	fmt.Println("WASM Search Engine Initializing...")

	// Expose functions to JS
	js.Global().Set("initSearch", js.FuncOf(initSearch))
	js.Global().Set("searchPosts", js.FuncOf(searchPosts))

	fmt.Println("WASM Search Engine Ready")
	<-c
}

func initSearch(this js.Value, args []js.Value) interface{} {
	if len(args) < 1 {
		return "Error: No URL provided"
	}
	url := args[0].String()

	handler := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		resolve := args[0]
		reject := args[1]

		go func() {
			data, err := fetchBytes(url)
			if err != nil {
				reject.Invoke(fmt.Sprintf("Fetch error: %v", err))
				return
			}

			dec := gob.NewDecoder(bytes.NewReader(data))
			if err := dec.Decode(&index); err != nil {
				reject.Invoke(fmt.Sprintf("Decode error: %v", err))
				return
			}

			resolve.Invoke(len(index.Posts))
		}()

		return nil
	})

	promiseConstructor := js.Global().Get("Promise")
	return promiseConstructor.New(handler)
}

func fetchBytes(url string) ([]byte, error) {
	ch := make(chan interface{}, 1)

	window := js.Global()
	promise := window.Call("fetch", url)

	success := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		resp := args[0]
		if !resp.Get("ok").Bool() {
			ch <- fmt.Errorf("bad status: %s", resp.Get("statusText").String())
			return nil
		}

		bufPromise := resp.Call("arrayBuffer")
		bufSuccess := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
			buf := args[0]
			uint8Array := js.Global().Get("Uint8Array").New(buf)
			dst := make([]byte, uint8Array.Length())
			js.CopyBytesToGo(dst, uint8Array)
			ch <- dst
			return nil
		})
		bufPromise.Call("then", bufSuccess)
		return nil
	})

	failure := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		ch <- fmt.Errorf("fetch failed")
		return nil
	})

	promise.Call("then", success, failure)

	result := <-ch
	if err, ok := result.(error); ok {
		return nil, err
	}
	return result.([]byte), nil
}

func searchPosts(this js.Value, args []js.Value) interface{} {
	if len(args) < 1 {
		return nil
	}
	query := args[0].String()

	results := search.PerformSearch(&index, query)

	// Convert to JS objects
	finalResults := make([]interface{}, 0)
	for _, res := range results {
		jsRes := make(map[string]interface{})
		jsRes["title"] = res.Title
		jsRes["link"] = res.Link
		jsRes["description"] = res.Description
		jsRes["snippet"] = res.Snippet
		jsRes["score"] = res.Score
		finalResults = append(finalResults, jsRes)
	}

	return js.ValueOf(finalResults)
}
