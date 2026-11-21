// Package limits holds build-wide numeric limits that low-level infrastructure
// packages need.
//
// It must stay dependency-free: pools and async are linked into the WASM search
// build, and pulling in builder/models (which imports the D2 engine and
// html/template) blows that binary up by megabytes.
package limits

const (
	// MaxBufferSize is the maximum buffer size for stream operations.
	MaxBufferSize = 64 * 1024 // 64KB
	// MaxWorkers is the upper bound on worker count.
	MaxWorkers = 32
	// WorkerBufferSize is the channel buffer size per worker.
	WorkerBufferSize = 4
)
