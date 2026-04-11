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

// clientSlicePool stores *[]chan<- string slices for broadcast snapshots.
var clientSlicePool = sync.Pool{
	New: func() any {
		s := make([]chan<- string, 0, clientSliceCap)
		return &s
	},
}

var (
	clientMu sync.Mutex
	clients  = make(map[chan string]struct{})
)

func handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	clientChan := make(chan string, clientChannelBuffer)
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

	heartbeat := time.NewTicker(30 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case target := <-clientChan:
			_, _ = fmt.Fprintf(w, "data: %s\n\n", target)
			flusher.Flush()
		case <-heartbeat.C:
			// Send SSE comment as a heartbeat to keep connection alive
			_, _ = fmt.Fprintf(w, ": heartbeat\n\n")
			flusher.Flush()
		}
	}
}

func BroadcastReload(target, path string) {
	// Wait for active build to complete before broadcasting for 'site' or 'all'
	if target != "root" {
		if waitCh := waitForBuild(); waitCh != nil {
			<-waitCh
		}
	}

	clientMu.Lock()
	// Use pooled slice for snapshot
	slicePtr := clientSlicePool.Get().(*[]chan<- string)
	clientsSnapshot := (*slicePtr)[:0]
	for client := range clients {
		clientsSnapshot = append(clientsSnapshot, client)
	}
	clientMu.Unlock()

	for _, clientChan := range clientsSnapshot {
		select {
		case clientChan <- target:
		default:
			// Client channel full; skip
		}
	}

	// Return slice to pool
	*slicePtr = (*slicePtr)[:0]
	clientSlicePool.Put(slicePtr)
}
