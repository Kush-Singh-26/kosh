//go:build js && wasm
// +build js,wasm

package main

import (
	"bytes"
	"fmt"
	"io"
	"syscall/js"

	"github.com/andybalholm/brotli"

	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/search"
)

var index models.SearchIndex

func main() {
	c := make(chan struct{}, 0)
	println("WASM Search Engine Initializing (Schema v7)...")

	js.Global().Set("initSearch", js.FuncOf(initSearch))
	js.Global().Set("searchPosts", js.FuncOf(searchPosts))

	println("WASM Search Engine Ready")
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
			data, err := fetchAndDecompress(url)
			if err != nil {
				reject.Invoke("Fetch/Decompress error: " + err.Error())
				return
			}

			if _, err := index.UnmarshalMsg(data); err != nil {
				reject.Invoke("Decode error: " + err.Error())
				return
			}

			// Validate schema version
			if index.SchemaVersion != models.CurrentSchemaVersion {
				reject.Invoke("Incompatible index schema: please rebuild your site")
				return
			}

			resolve.Invoke(len(index.Posts))
		}()

		return nil
	})

	promiseConstructor := js.Global().Get("Promise")
	promise := promiseConstructor.New(handler)
	handler.Release()
	return promise
}

func fetchAndDecompress(url string) ([]byte, error) {
	ch := make(chan interface{}, 1)

	window := js.Global()
	promise := window.Call("fetch", url)

	var success js.Func
	var failure js.Func
	success = js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		defer success.Release()
		defer failure.Release()

		resp := args[0]
		if !resp.Get("ok").Bool() {
			ch <- "bad status: " + resp.Get("statusText").String()
			return nil
		}

		bufPromise := resp.Call("arrayBuffer")
		var bufSuccess js.Func
		var bufFailure js.Func
		bufSuccess = js.FuncOf(func(this js.Value, args []js.Value) interface{} {
			defer bufSuccess.Release()
			defer bufFailure.Release()

			buf := args[0]
			uint8Array := window.Get("Uint8Array").New(buf)
			compressed := make([]byte, uint8Array.Length())
			js.CopyBytesToGo(compressed, uint8Array)

			// Log some bytes for debugging
			hexStr := ""
			for k := 0; k < len(compressed) && k < 16; k++ {
				hexStr += fmt.Sprintf("%02X ", compressed[k])
			}
			println("WASM: Data fetched size:", len(compressed), "bytes. First 16 bytes:", hexStr)

			// Decompress in Go with Brotli
			br := brotli.NewReader(bytes.NewReader(compressed))
			decompressed, err := io.ReadAll(br)
			if err != nil {
				println("WASM: Brotli decompression error:", err.Error(), "- using raw data fallback")
				ch <- compressed
				return nil
			}

			println("WASM: Successfully decompressed", len(decompressed), "bytes")
			ch <- decompressed
			return nil
		})
		bufFailure = js.FuncOf(func(this js.Value, args []js.Value) interface{} {
			defer bufSuccess.Release()
			defer bufFailure.Release()

			ch <- "failed to read array buffer"
			return nil
		})
		bufPromise.Call("then", bufSuccess, bufFailure)
		return nil
	})

	failure = js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		defer success.Release()
		defer failure.Release()

		ch <- "fetch failed"
		return nil
	})

	promise.Call("then", success, failure)

	result := <-ch
	if s, ok := result.(string); ok {
		return nil, &jsError{msg: s}
	}
	return result.([]byte), nil
}

type jsError struct {
	msg string
}

func (e *jsError) Error() string {
	return e.msg
}

func searchPosts(this js.Value, args []js.Value) interface{} {
	if len(args) < 1 {
		return nil
	}
	query := args[0].String()
	versionFilter := ""
	if len(args) >= 2 {
		versionFilter = args[1].String()
	}

	results := search.PerformSearch(&index, query, versionFilter)

	finalResults := make([]interface{}, 0, len(results))
	for _, res := range results {
		jsRes := make(map[string]interface{})
		jsRes["title"] = res.Title
		jsRes["link"] = res.Link
		jsRes["description"] = res.Description
		jsRes["snippet"] = res.Snippet
		jsRes["version"] = res.Version
		jsRes["score"] = res.Score
		finalResults = append(finalResults, jsRes)
	}

	return js.ValueOf(finalResults)
}
