package server

import (
	"fmt"
	"net/http"
)

func handleSSE(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	clientChan := make(chan struct{})
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
		clientMu.Lock()
		for clientChan := range clients {
			select {
			case clientChan <- struct{}{}:
			default:
			}
		}
		clientMu.Unlock()
	}
}
