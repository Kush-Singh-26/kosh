package utils

import (
	"runtime"
)

// Default constants - these are used as fallbacks
// Actual values come from BuildConfig loaded from kosh.build.yaml
const (
	MaxBufferSize       = 64 * 1024 // 64KB
	InlineHTMLThreshold = 32 * 1024 // 32KB
	RawThreshold        = 512
	FastZstdMax         = 64 * 1024        // 64KB
	MaxFileSize         = 50 * 1024 * 1024 // 50MB
)

// Legacy constant for backward compatibility
const DefaultWorkerCountMax = 12

// GetDefaultWorkerCount returns the default worker count based on CPU cores
func GetDefaultWorkerCount() int {
	workers := runtime.NumCPU()
	if workers < 2 {
		return 2
	}
	if workers > DefaultWorkerCountMax {
		return DefaultWorkerCountMax
	}
	return workers
}
