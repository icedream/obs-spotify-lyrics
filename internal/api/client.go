package api

import (
	"github.com/gorilla/websocket"
)

type wsClient struct {
	conn *websocket.Conn
	send chan []byte   // buffered outbound message queue
	done chan struct{} // closed when the client is unsubscribed
}

// readPump drains any inbound messages (the widget never sends any) and
// returns when the connection is closed.
func (c *wsClient) readPump() {
	defer func() { _ = c.conn.Close() }()
	c.conn.SetReadLimit(512)
	for {
		if _, _, err := c.conn.ReadMessage(); err != nil {
			break
		}
	}
}

// writePump forwards queued messages to the WebSocket connection.
func (c *wsClient) writePump() {
	defer func() { _ = c.conn.Close() }()
	for {
		select {
		case m := <-c.send:
			if err := c.conn.WriteMessage(websocket.TextMessage, m); err != nil {
				return
			}
		case <-c.done:
			return
		}
	}
}
