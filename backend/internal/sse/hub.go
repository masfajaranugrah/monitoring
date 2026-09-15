package sse

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// Hub broadcasts realtime events to connected SSE clients.
type Hub struct {
	mu       sync.RWMutex
	clients  map[chan []byte]struct{}
	messages chan event

	// Broadcasts can be Rate-limited in heavy deployments.
	queueSize int
}

type event struct {
	Event string      `json:"event"`
	Data  interface{} `json:"data"`
}

func NewHub() *Hub {
	h := &Hub{
		clients:  make(map[chan []byte]struct{}),
		messages: make(chan event, 256),
		queueSize: 256,
	}
	go h.run()
	return h
}

func (h *Hub) run() {
	for ev := range h.messages {
		b, err := json.Marshal(ev)
		if err != nil {
			continue
		}
		payload := append([]byte("data: "), b...)
		payload = append(payload, '\n', '\n')

		h.mu.RLock()
		for ch := range h.clients {
			select {
			case ch <- payload:
			default:
				log.Println("[sse] dropping event for slow client")
			}
		}
		h.mu.RUnlock()
	}
}

func (h *Hub) subscribe() chan []byte {
	ch := make(chan []byte, h.queueSize)
	h.mu.Lock()
	h.clients[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *Hub) unsubscribe(ch chan []byte) {
	h.mu.Lock()
	delete(h.clients, ch)
	h.mu.Unlock()
}

// ---- EventPublisher interface (used by the monitor engine) ----

func (h *Hub) BroadcastCustomerUpdate(payload map[string]interface{}) {
	h.messages <- event{Event: "customer:update", Data: payload}
}

func (h *Hub) BroadcastStats(payload interface{}) {
	h.messages <- event{Event: "stats:update", Data: payload}
}

func (h *Hub) BroadcastVPNStatus(payload map[string]interface{}) {
	h.messages <- event{Event: "vpn:update", Data: payload}
}

// PublishAlert emits an alert notification event.
func (h *Hub) PublishAlert(payload interface{}) {
	h.messages <- event{Event: "alert:new", Data: payload}
}

// Stream is the SSE endpoint handler (GET /api/events).
func (h *Hub) Stream(c *gin.Context) {
	flusher, ok := c.Writer.(gin.ResponseWriter)
	if !ok {
		c.JSON(500, gin.H{"error": "streaming unsupported"})
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	ch := h.subscribe()
	defer h.unsubscribe(ch)

	// Send a heartbeat + initial ping so the connection is verified immediately.
	hb, _ := json.Marshal(event{Event: "hello", Data: map[string]interface{}{"time": time.Now()}})
	_, _ = c.Writer.WriteString("data: " + string(hb) + "\n\n")
	flusher.Flush()

	clientGone := c.Request.Context().Done()
	keepAlive := time.NewTicker(15 * time.Second)
	defer keepAlive.Stop()

	for {
		select {
		case <-clientGone:
			return
		case <-keepAlive.C:
			_, _ = c.Writer.WriteString(": ping\n\n")
			flusher.Flush()
		case msg := <-ch:
			_, _ = c.Writer.Write(msg)
			flusher.Flush()
		}
	}
}