package ws

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// Event is a single realtime message pushed to clients. It keeps the same
// shape as the old SSE payload ({ "event": ..., "data": ... }) so the frontend
// contract is unchanged.
type Event struct {
	Event string      `json:"event"`
	Data  interface{} `json:"data"`
}

// Hub broadcasts realtime events to connected WebSocket clients.
type Hub struct {
	mu       sync.RWMutex
	clients  map[*client]struct{}
	messages chan Event
}

type client struct {
	hub  *Hub
	conn *websocket.Conn
	send chan []byte
}

var upgrader = websocket.Upgrader{
	CheckOrigin:     func(r *http.Request) bool { return true },
	ReadBufferSize:  1024,
	WriteBufferSize: 4096,
}

func NewHub() *Hub {
	h := &Hub{
		clients:  make(map[*client]struct{}),
		messages: make(chan Event, 256),
	}
	go h.run()
	return h
}

// run serializes all event marshalling + fan-out so writers stay as the only
// goroutine touching each client's send queue ordering.
func (h *Hub) run() {
	for ev := range h.messages {
		b, err := json.Marshal(ev)
		if err != nil {
			continue
		}
		h.broadcast(b)
	}
}

func (h *Hub) BroadcastCustomerUpdate(payload map[string]interface{}) {
	h.messages <- Event{Event: "customer:update", Data: payload}
}

func (h *Hub) BroadcastStats(payload interface{}) {
	h.messages <- Event{Event: "stats:update", Data: payload}
}

func (h *Hub) BroadcastVPNStatus(payload map[string]interface{}) {
	h.messages <- Event{Event: "vpn:update", Data: payload}
}

// PublishAlert emits an alert notification event.
func (h *Hub) PublishAlert(payload interface{}) {
	h.messages <- Event{Event: "alert:new", Data: payload}
}

func (h *Hub) register(cl *client) {
	h.mu.Lock()
	h.clients[cl] = struct{}{}
	h.mu.Unlock()
}

func (h *Hub) unregister(cl *client) {
	h.mu.Lock()
	if _, ok := h.clients[cl]; ok {
		delete(h.clients, cl)
		close(cl.send)
	}
	h.mu.Unlock()
}

func (h *Hub) broadcast(b []byte) {
	h.mu.RLock()
	for cl := range h.clients {
		select {
		case cl.send <- b:
		default:
			// Slow/stuck client: kick it out. Run async so we never hold the
			// RLock while taking the write lock in unregister.
			go func(c *client) {
				c.hub.unregister(c)
				_ = c.conn.Close()
			}(cl)
		}
	}
	h.mu.RUnlock()
}

// Stream is the WebSocket endpoint handler (GET /api/events).
func (h *Hub) Stream(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}

	cl := &client{
		hub:  h,
		conn: conn,
		send: make(chan []byte, 256),
	}
	h.register(cl)

	// Send a hello immediately so the connection is verified right away.
	if hello, err := json.Marshal(Event{
		Event: "hello",
		Data:  map[string]interface{}{"time": time.Now()},
	}); err == nil {
		select {
		case cl.send <- hello:
		default:
		}
	}

	go cl.writePump()

	// readPump blocks until the client goes away or the connection breaks;
	// its deferred cleanup removes the client when this handler returns.
	cl.readPump()
	h.unregister(cl)
	_ = conn.Close()
}

// writePump writes queued events and periodic pings. Closing the send channel
// (via unregister) makes it exit cleanly.
func (cl *client) writePump() {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		cl.hub.unregister(cl)
		_ = cl.conn.Close()
	}()

	for {
		select {
		case payload, ok := <-cl.send:
			if !ok {
				return
			}
			_ = cl.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := cl.conn.WriteMessage(websocket.TextMessage, payload); err != nil {
				return
			}
		case <-ticker.C:
			_ = cl.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := cl.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// readPump drains incoming messages. We never expect client data, but reading
// is what lets gorilla auto-respond to pings and detect dead sockets. A read
// deadline + pong handler keeps half-open connections from lingering.
func (cl *client) readPump() {
	defer func() {
		cl.hub.unregister(cl)
		_ = cl.conn.Close()
	}()

	cl.conn.SetReadLimit(4096)
	_ = cl.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	cl.conn.SetPongHandler(func(string) error {
		return cl.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	})

	for {
		if _, _, err := cl.conn.ReadMessage(); err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Printf("[ws] read error: %v", err)
			}
			return
		}
	}
}
