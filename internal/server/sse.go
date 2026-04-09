package server

import (
	"fmt"
	"net/http"
	"sync"
	"time"
)

const (
	clientSliceCap      = 16
	clientChannelBuffer = 5
)

// clientSlicePool stores *[]chan<- struct{} slices for broadcast snapshots.
var clientSlicePool = sync.Pool{
	New: func() any {
		s := make([]chan<- struct{}, 0, clientSliceCap)
		return &s
	},
}

func handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// Increased buffer to 5 to handle rapid reload events
	clientChan := make(chan struct{}, clientChannelBuffer)
	clientMu.Lock()
	clients[clientChan] = struct{}{}
	clientMu.Unlock()

	defer func() {
		clientMu.Lock()
		delete(clients, clientChan)
		clientMu.Unlock()
	}()

	_, _ = fmt.Fprintf(w, "data: connected\n\n")
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-clientChan:
			_, _ = fmt.Fprintf(w, "data: reload\n\n")
			flusher.Flush()
		}
	}
}

func broadcastReload(ch <-chan struct{}) {
	var lastReload time.Time
	for range ch {
		// Throttling: only reload once every 500ms to avoid refresh loops during rapid changes
		if time.Since(lastReload) < 500*time.Millisecond {
			continue
		}
		lastReload = time.Now()

		// Wait for active build to complete before broadcasting
		if waitCh := waitForBuild(); waitCh != nil {
			<-waitCh
		}

		clientMu.Lock()
		// Use pooled slice for snapshot
		slicePtr := clientSlicePool.Get().(*[]chan<- struct{})
		clientsSnapshot := (*slicePtr)[:0]
		for client := range clients {
			clientsSnapshot = append(clientsSnapshot, client)
		}
		clientMu.Unlock()

		for _, clientChan := range clientsSnapshot {
			select {
			case clientChan <- struct{}{}:
			default:
				// Client channel full; skip — channel already has buffer 5
			}
		}

		// Return slice to pool (safe to reuse after clearing)
		*slicePtr = (*slicePtr)[:0]
		clientSlicePool.Put(slicePtr)
	}
}
