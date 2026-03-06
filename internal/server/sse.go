package server

import (
	"fmt"
	"net/http"
	"time"
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

	// Increased buffer to 5 to handle rapid reload events
	clientChan := make(chan struct{}, 5)
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
	for range ch {
		// Wait for active build to complete before broadcasting
		if waitCh := waitForBuild(); waitCh != nil {
			<-waitCh
		}

		clientMu.Lock()
		clientsSnapshot := make([]chan<- struct{}, 0, len(clients))
		for client := range clients {
			clientsSnapshot = append(clientsSnapshot, client)
		}
		clientMu.Unlock()

		for _, clientChan := range clientsSnapshot {
			select {
			case clientChan <- struct{}{}:
			case <-time.After(100 * time.Millisecond):
				// Client slow, skip this time
			}
		}
	}
}
