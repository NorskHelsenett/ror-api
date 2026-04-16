package statuspage

import (
	"encoding/json"
	"io"
	"log"
	"sync"

	"github.com/gin-gonic/gin"
)

// SSEHub manages Server-Sent Event client connections and broadcasts.
type SSEHub struct {
	mu      sync.RWMutex
	clients map[chan []byte]struct{}
}

// NewSSEHub creates a new SSE hub.
func NewSSEHub() *SSEHub {
	return &SSEHub{
		clients: make(map[chan []byte]struct{}),
	}
}

// Subscribe registers a new client and returns its channel.
func (h *SSEHub) Subscribe() chan []byte {
	ch := make(chan []byte, 16)
	h.mu.Lock()
	h.clients[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

// Unsubscribe removes a client.
func (h *SSEHub) Unsubscribe(ch chan []byte) {
	h.mu.Lock()
	delete(h.clients, ch)
	close(ch)
	h.mu.Unlock()
}

// Broadcast sends a snapshot to all connected clients.
func (h *SSEHub) Broadcast(snapshot *StatusSnapshot) {
	data, err := json.Marshal(snapshot)
	if err != nil {
		log.Printf("error marshaling snapshot: %v", err)
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	for ch := range h.clients {
		select {
		case ch <- data:
		default:
			// Drop message if client is slow
		}
	}
}

// HandleSSE returns a Gin handler for the SSE endpoint.
func (h *SSEHub) HandleSSE(currentSnapshot func() *StatusSnapshot) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Header("X-Accel-Buffering", "no")

		ch := h.Subscribe()
		defer h.Unsubscribe(ch)

		// Send current state immediately on connect
		if snap := currentSnapshot(); snap != nil {
			data, err := json.Marshal(snap)
			if err == nil {
				c.SSEvent("status", string(data))
				c.Writer.Flush()
			}
		}

		c.Stream(func(w io.Writer) bool {
			select {
			case msg, ok := <-ch:
				if !ok {
					return false
				}
				c.SSEvent("status", string(msg))
				return true
			case <-c.Request.Context().Done():
				return false
			}
		})
	}
}
