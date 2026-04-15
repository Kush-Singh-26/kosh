//go:build !wasm

package assets

// embeddedWasmHash is the hash of the raw (decompressed) embedded WASM.
// It mirrors SearchWasmHash to keep legacy tests stable.
var embeddedWasmHash = SearchWasmHash

// wasmInitErr captures initialization errors for embedded WASM.
var wasmInitErr error
