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
	clients map[chan sseMessage]struct{}
}

type sseMessage struct {
	event string
	data  []byte
}

// NewSSEHub creates a new SSE hub.
func NewSSEHub() *SSEHub {
	return &SSEHub{
		clients: make(map[chan sseMessage]struct{}),
	}
}

// Subscribe registers a new client and returns its channel.
func (h *SSEHub) Subscribe() chan sseMessage {
	ch := make(chan sseMessage, 16)
	h.mu.Lock()
	h.clients[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

// Unsubscribe removes a client.
func (h *SSEHub) Unsubscribe(ch chan sseMessage) {
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
	h.send(sseMessage{event: "status", data: data})
}

// BroadcastEvent sends a named event with arbitrary JSON data.
func (h *SSEHub) BroadcastEvent(event string, payload interface{}) {
	data, err := json.Marshal(payload)
	if err != nil {
		log.Printf("error marshaling %s event: %v", event, err)
		return
	}
	h.send(sseMessage{event: event, data: data})
}

func (h *SSEHub) send(msg sseMessage) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.clients {
		select {
		case ch <- msg:
		default:
		}
	}
}

// HandleSSE returns a Gin handler for the SSE endpoint.
func (h *SSEHub) HandleSSE(currentSnapshot func() *StatusSnapshot, currentAlerts func() *AlertsResponse, currentMetrics func() *MetricsSnapshot) gin.HandlerFunc {
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
		if alerts := currentAlerts(); alerts != nil {
			data, err := json.Marshal(alerts)
			if err == nil {
				c.SSEvent("alerts", string(data))
				c.Writer.Flush()
			}
		}
		if metrics := currentMetrics(); metrics != nil {
			data, err := json.Marshal(metrics)
			if err == nil {
				c.SSEvent("metrics", string(data))
				c.Writer.Flush()
			}
		}

		c.Stream(func(w io.Writer) bool {
			select {
			case msg, ok := <-ch:
				if !ok {
					return false
				}
				c.SSEvent(msg.event, string(msg.data))
				return true
			case <-c.Request.Context().Done():
				return false
			}
		})
	}
}
