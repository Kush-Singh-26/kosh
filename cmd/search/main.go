//go:build js && wasm
// +build js,wasm

package main

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"strconv"
	"syscall/js"

	"github.com/andybalholm/brotli"

	"github.com/Kush-Singh-26/kosh/builder/async"
	"github.com/Kush-Singh-26/kosh/builder/models"
	"github.com/Kush-Singh-26/kosh/builder/search"
)

var (
	index       models.SearchIndex
	lastQuery   string
	lastResults []any
)

func main() {
	c := make(chan struct{}, 0)
	println("WASM Search Engine Initializing (Schema v" + strconv.Itoa(models.CurrentSchemaVersion) + ")...")

	js.Global().Set("initSearch", js.FuncOf(initSearch))
	js.Global().Set("searchItems", js.FuncOf(searchItems))
	js.Global().Set("getSuggestions", js.FuncOf(getSuggestions))

	println("WASM Search Engine Ready")
	<-c
}

func getSuggestions(this js.Value, args []js.Value) any {
	if len(args) < 1 {
		return nil
	}
	prefix := args[0].String()
	suggestions := search.GetSuggestions(&index, prefix)

	jsSug := make([]any, 0, len(suggestions))
	for _, s := range suggestions {
		jsSug = append(jsSug, s)
	}

	return js.ValueOf(jsSug)
}

func initSearch(this js.Value, args []js.Value) any {
	if len(args) < 1 {
		return "Error: No URL provided"
	}
	url := args[0].String()

	handler := js.FuncOf(func(this js.Value, args []js.Value) any {
		resolve := args[0]
		reject := args[1]

		async.FireAndForget(context.Background(), slog.Default(), "wasm search init", func() error {
			data, err := fetchAndDecompress(url)
			if err != nil {
				reject.Invoke("Fetch/Decompress error: " + err.Error())
				return nil
			}

			if _, err := index.UnmarshalMsg(data); err != nil {
				reject.Invoke("Decode error: " + err.Error())
				return nil
			}

			// Validate schema version
			if index.SchemaVersion != models.CurrentSchemaVersion {
				reject.Invoke("Incompatible index schema: please rebuild your site")
				return nil
			}

			// Clear cache on new index load
			lastQuery = ""
			lastResults = nil

			resolve.Invoke(index.TotalItems)
			return nil
		})

		return nil
	})

	promiseConstructor := js.Global().Get("Promise")
	promise := promiseConstructor.New(handler)
	handler.Release()
	return promise
}

func fetchAndDecompress(url string) ([]byte, error) {
	ch := make(chan any, 1)

	window := js.Global()
	promise := window.Call("fetch", url)

	var success js.Func
	var failure js.Func
	success = js.FuncOf(func(this js.Value, args []js.Value) any {
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

		bufSuccess = js.FuncOf(func(this js.Value, args []js.Value) any {
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

		bufFailure = js.FuncOf(func(this js.Value, args []js.Value) any {
			defer bufSuccess.Release()
			defer bufFailure.Release()
			ch <- "arrayBuffer failed"
			return nil
		})

		bufPromise.Call("then", bufSuccess, bufFailure)
		return nil
	})

	failure = js.FuncOf(func(this js.Value, args []js.Value) any {
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

func searchItems(this js.Value, args []js.Value) any {
	if len(args) < 1 {
		return nil
	}
	query := args[0].String()

	// Simple exact/prefix cache
	if query == lastQuery && lastResults != nil {
		return js.ValueOf(lastResults)
	}

	results := search.PerformSearch(&index, query)

	finalResults := make([]any, 0, len(results))
	for _, res := range results {
		// jsRes is JSON-compatible: strings for text fields, float64 for score.
		jsRes := make(map[string]any)
		jsRes["title"] = res.Title
		jsRes["link"] = res.Link
		jsRes["description"] = res.Description
		jsRes["snippet"] = res.Snippet
		jsRes["score"] = res.Score

		// Convert taxonomies to any map for JSValueOf
		jsTax := make(map[string]any)
		for k, v := range res.Taxonomies {
			terms := make([]any, len(v))
			for i, t := range v {
				terms[i] = t
			}
			jsTax[k] = terms
		}
		jsRes["taxonomies"] = jsTax

		finalResults = append(finalResults, jsRes)
	}

	lastQuery = query
	lastResults = finalResults

	return js.ValueOf(finalResults)
}
