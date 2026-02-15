package utils

import "runtime"

const (
	MaxBufferSize = 64 * 1024

	InlineHTMLThreshold = 32 * 1024

	RawThreshold = 512

	FastZstdMax = 64 * 1024
)

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

const DefaultWorkerCountMax = 12
