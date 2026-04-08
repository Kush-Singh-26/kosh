//go:build js && wasm
// +build js,wasm

package main

import (
	"bytes"
	"io"
	"strconv"
	"syscall/js"

	"github.com/andybalholm/brotli"

	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/search"
)

var (
	index       models.SearchIndex
	lastQuery   string
	lastResults []interface{}
)

func main() {
	c := make(chan struct{}, 0)
	println("WASM Search Engine Initializing (Schema v" + strconv.Itoa(models.CurrentSchemaVersion) + ")...")

	js.Global().Set("initSearch", js.FuncOf(initSearch))
	js.Global().Set("searchPosts", js.FuncOf(searchPosts))
	js.Global().Set("getSuggestions", js.FuncOf(getSuggestions))

	println("WASM Search Engine Ready")
	<-c
}

func getSuggestions(this js.Value, args []js.Value) interface{} {
	if len(args) < 1 {
		return nil
	}
	prefix := args[0].String()
	suggestions := search.GetSuggestions(&index, prefix)

	jsSug := make([]interface{}, 0, len(suggestions))
	for _, s := range suggestions {
		jsSug = append(jsSug, s)
	}

	return js.ValueOf(jsSug)
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

			// Clear cache on new index load
			lastQuery = ""
			lastResults = nil

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

			if len(args) < 1 {
				ch <- "no buffer data"
				return nil
			}

			arrayBuffer := args[0]
			uint8Array := window.Get("Uint8Array").New(arrayBuffer)
			length := uint8Array.Get("length").Int()
			data := make([]byte, length)
			js.CopyBytesToGo(data, uint8Array)

			br := brotli.NewReader(bytes.NewReader(data))
			decompressed, err := io.ReadAll(br)
			if err != nil {
				ch <- "decompression failed: " + err.Error()
				return nil
			}

			ch <- decompressed
			return nil
		})

		bufFailure = js.FuncOf(func(this js.Value, args []js.Value) interface{} {
			defer bufSuccess.Release()
			defer bufFailure.Release()
			ch <- "arrayBuffer failed"
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

// Error implements the error interface.
func (e *jsError) Error() string {
	return e.msg
}

func searchPosts(this js.Value, args []js.Value) interface{} {
	if len(args) < 1 {
		return nil
	}
	query := args[0].String()

	// Simple exact/prefix cache
	if query == lastQuery && lastResults != nil {
		return js.ValueOf(lastResults)
	}

	results := search.PerformSearch(&index, query)

	finalResults := make([]interface{}, 0, len(results))
	for _, res := range results {
		jsRes := make(map[string]interface{})
		jsRes["title"] = res.Title
		jsRes["link"] = res.Link
		jsRes["description"] = res.Description
		jsRes["snippet"] = res.Snippet
		jsRes["score"] = res.Score
		finalResults = append(finalResults, jsRes)
	}

	lastQuery = query
	lastResults = finalResults

	return js.ValueOf(finalResults)
}
