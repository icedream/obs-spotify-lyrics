package api

import (
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

// hub tracks all connected WebSocket clients and broadcasts state to them.
type hub struct {
	mu      sync.Mutex
	clients map[*wsClient]struct{}
	last    []byte // most recent broadcast, sent to newly connecting clients
}

func newHub() *hub {
	return &hub{clients: make(map[*wsClient]struct{})}
}

func (h *hub) subscribe(c *wsClient) {
	h.mu.Lock()
	h.clients[c] = struct{}{}
	last := h.last
	h.mu.Unlock()
	// Immediately send the current state so the widget doesn't start blank.
	if last != nil {
		select {
		case c.send <- last:
		default:
		}
	}
}

func (h *hub) unsubscribe(c *wsClient) {
	h.mu.Lock()
	_, present := h.clients[c]
	if present {
		delete(h.clients, c)
	}
	h.mu.Unlock()
	if present {
		close(c.done) // signals writePump to exit
	}
}

func (h *hub) broadcast(payload []byte) {
	h.mu.Lock()
	h.last = payload
	// Snapshot the client list under the lock so sends happen outside it.
	clients := make([]*wsClient, 0, len(h.clients))
	for c := range h.clients {
		clients = append(clients, c)
	}
	h.mu.Unlock()
	for _, c := range clients {
		select {
		case c.send <- payload:
		default:
			// Slow client, drop this message; it will resync on the next poll.
		}
	}
}

var wsUpgrader = websocket.Upgrader{
	// Allow all origins so OBS can load the page from any address.
	CheckOrigin: func(r *http.Request) bool { return true },
}

func (h *hub) wsHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := wsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		c := &wsClient{
			conn: conn,
			send: make(chan []byte, 8),
			done: make(chan struct{}),
		}
		h.subscribe(c)
		defer h.unsubscribe(c)
		go c.writePump()
		c.readPump() // blocks until the client disconnects
	}
}
