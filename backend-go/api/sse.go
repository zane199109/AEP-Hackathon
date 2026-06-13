package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// SSEEvent represents a server-sent event to the frontend topology.
type SSEEvent struct {
	Type      string `json:"type"`      // "bounty_posted", "claimed", "submitted", "confirmed", "settled"
	JobID     uint64 `json:"job_id"`
	Status    string `json:"status"`    // "Open", "Assigned", "Submitted", "Verified", "Slashed", "Refunded"
	Color     string `json:"color"`     // "blue", "green", "red", "yellow" for topology pulse
	Message   string `json:"message,omitempty"`
	Timestamp string `json:"timestamp"`
}

// SSEHub manages Server-Sent Event connections.
type SSEHub struct {
	mu      sync.RWMutex
	clients map[chan SSEEvent]struct{}
}

// NewSSEHub creates a new SSE hub.
func NewSSEHub() *SSEHub {
	return &SSEHub{
		clients: make(map[chan SSEEvent]struct{}),
	}
}

// Subscribe adds a new client channel.
func (h *SSEHub) Subscribe() chan SSEEvent {
	h.mu.Lock()
	defer h.mu.Unlock()
	ch := make(chan SSEEvent, 256)
	h.clients[ch] = struct{}{}
	return ch
}

// Unsubscribe removes a client channel.
func (h *SSEHub) Unsubscribe(ch chan SSEEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.clients, ch)
	close(ch)
}

// Emit sends an event to all connected clients.
// Blocks with a 100ms timeout per client to avoid dropping events.
func (h *SSEHub) Emit(evt SSEEvent) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	evt.Timestamp = time.Now().UTC().Format(time.RFC3339)
	for ch := range h.clients {
		select {
		case ch <- evt:
		case <-time.After(100 * time.Millisecond):
			// Client is too slow, skip to next
		}
	}
}

// pushEvent creates an SSEEvent from action parameters and emits it.
func (h *SSEHub) pushEvent(eventType string, jobID uint64, status, color, message string) {
	h.Emit(SSEEvent{
		Type:    eventType,
		JobID:   jobID,
		Status:  status,
		Color:   color,
		Message: message,
	})
}

// SSEHandler handles GET /api/events — SSE stream for the React topology display.
func (h *SSEHub) SSEHandler(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	ch := h.Subscribe()
	defer h.Unsubscribe(ch)

	// Send initial connection event
	fmt.Fprintf(w, "event: connected\ndata: {\"status\":\"connected\"}\n\n")
	flusher.Flush()

	for {
		select {
		case evt, ok := <-ch:
			if !ok {
				return
			}
			data, _ := json.Marshal(evt)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", evt.Type, data)
			flusher.Flush()
		case <-time.After(15 * time.Second):
			// Keepalive: send a comment to prevent proxy/browser timeout
			fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}
