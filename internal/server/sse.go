package server

import (
	"fmt"
	"net/http"
	"time"
)

func handleSSE(w http.ResponseWriter, r *http.Request) {
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
	w.(http.Flusher).Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-clientChan:
			_, _ = fmt.Fprintf(w, "data: reload\n\n")
			w.(http.Flusher).Flush()
		}
	}
}

func broadcastReload() {
	for range reloadChan {
		// Wait for active build to complete before broadcasting
		buildMu.Lock()
		for buildActive {
			buildComplete.Wait()
		}
		buildMu.Unlock()

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
