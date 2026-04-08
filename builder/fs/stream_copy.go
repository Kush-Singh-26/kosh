package fs

import (
	"io"
	"os"
	"sync"
)

// streamCopyBufferPool stores *[]byte buffers for streaming file copies.
var streamCopyBufferPool = sync.Pool{
	New: func() any {
		buf := make([]byte, 64*1024)
		return &buf
	},
}

// StreamCopyFile copies a file using a pooled buffer.
func StreamCopyFile(src, dst string) error {
	s, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = s.Close() }()

	d, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() { _ = d.Close() }()

	bufPtr := streamCopyBufferPool.Get().(*[]byte)
	defer streamCopyBufferPool.Put(bufPtr)

	_, err = io.CopyBuffer(d, s, *bufPtr)
	return err
}
