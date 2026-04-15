package server

import (
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

const (
	clientSliceCap      = 16
	clientChannelBuffer = 5
	maxSSEClients       = 1000
)

// clientSlicePool stores *[]chan<- string slices for broadcast snapshots.
var clientSlicePool = sync.Pool{
	New: func() any {
		s := make([]chan<- string, 0, clientSliceCap)
		return &s
	},
}

var (
	clientMu    sync.Mutex
	clientCount atomic.Int32
	clients     = make(map[chan string]struct{})
)

func handleSSE(w http.ResponseWriter, r *http.Request) {
	// Check client limit before proceeding
	if clientCount.Load() >= maxSSEClients {
		http.Error(w, "Server at capacity", http.StatusServiceUnavailable)
		return
	}

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
	clientCount.Add(1)
	clientMu.Unlock()

	defer func() {
		clientMu.Lock()
		delete(clients, clientChan)
		clientCount.Add(-1)
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

// BroadcastReload sends reload events to connected SSE clients.
func BroadcastReload(target string, _ string) {
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
