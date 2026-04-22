//go:build js && wasm
// +build js,wasm

package main

import (
	"strconv"
	"syscall/js"

	"github.com/Kush-Singh-26/kosh/builder/models/searchpkg"
	"github.com/Kush-Singh-26/kosh/builder/search"
)

var (
	index       searchpkg.SearchIndex
	lastQuery   string
	lastResults []any
)

func main() {
	c := make(chan struct{}, 0)
	println("WASM Search Engine Initializing (Schema v" + strconv.Itoa(searchpkg.CurrentSchemaVersion) + ")...")

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
		return "Error: No data provided"
	}

	// Expecting a Uint8Array from JS
	uint8Array := args[0]
	length := uint8Array.Get("length").Int()
	data := make([]byte, length)
	js.CopyBytesToGo(data, uint8Array)

	var newIndex searchpkg.SearchIndex
	if _, err := newIndex.UnmarshalMsg(data); err != nil {
		return "Decode error: " + err.Error()
	}

	// Validate schema version
	if newIndex.SchemaVersion != int64(searchpkg.CurrentSchemaVersion) {
		return "Incompatible index schema: please rebuild your site"
	}

	// Replace global index
	index = newIndex

	// Clear cache on new index load
	lastQuery = ""
	lastResults = nil

	return index.TotalItems
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
		jsRes := make(map[string]any)
		jsRes["title"] = res.Title
		jsRes["link"] = res.Link
		jsRes["description"] = res.Description
		jsRes["snippet"] = res.Snippet
		jsRes["score"] = res.Score

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
