package main

import (
	"bytes"
	"fmt"

	"github.com/vmihailenco/msgpack/v5"
)

type OldIndex struct {
	DocLens  map[int]int                 `msgpack:"lens"`
	Inverted map[string]map[int][]int    `msgpack:"inv"`
}

type NewIndex struct {
	DocLens  map[string]int                 `msgpack:"lens"`
	Inverted map[string]map[string][]int    `msgpack:"inv"`
}

func main() {
	// Encode new, decode old
	newIdx := NewIndex{
		DocLens: map[string]int{"10": 5},
		Inverted: map[string]map[string][]int{
			"hello": {"10": []int{1, 2}},
		},
	}
	var buf bytes.Buffer
	enc := msgpack.NewEncoder(&buf)
	enc.Encode(&newIdx)

	var oldIdx OldIndex
	dec := msgpack.NewDecoder(bytes.NewReader(buf.Bytes()))
	err := dec.Decode(&oldIdx)
	fmt.Println("Encode NEW, Decode OLD:", err)

	// Encode old, decode new
	oldIdx2 := OldIndex{
		DocLens: map[int]int{10: 5},
		Inverted: map[string]map[int][]int{
			"hello": {10: []int{1, 2}},
		},
	}
	var buf2 bytes.Buffer
	enc2 := msgpack.NewEncoder(&buf2)
	enc2.Encode(&oldIdx2)

	var newIdx2 NewIndex
	dec2 := msgpack.NewDecoder(bytes.NewReader(buf2.Bytes()))
	err2 := dec2.Decode(&newIdx2)
	fmt.Println("Encode OLD, Decode NEW:", err2)
}
