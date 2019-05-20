package websocket

import (
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// Connection represents a websocket connection
type Connection struct {
	conn *websocket.Conn
}

// NewConnection creates a new connection
func NewConnection() *Connection {
	return &Connection{}
}

// Close sends the websocket CloseMessage
// https://tools.ietf.org/html/rfc6455#section-5.5.1
// graceful == false closes immediatly
func (c *Connection) Close(writeWait time.Duration, graceful bool) {
	if !graceful {
		c.conn.Close()
		return
	}

	c.conn.SetWriteDeadline(time.Now().Add(writeWait))
	c.conn.WriteMessage(websocket.CloseMessage, []byte{})

}

// Write to the network connection
func (c *Connection) Write(writeWait time.Duration, data []byte) error {
	c.conn.SetWriteDeadline(time.Now().Add(writeWait))

	c.conn.WriteMessage(websocket.CloseMessage, data)
	writer, err := c.conn.NextWriter(websocket.BinaryMessage)
	if err != nil {
		return err
	}
	writer.Write(data)

	// Flush data to the network
	if err := writer.Close(); err != nil {
		return err
	}

	return nil
}

// Ping sends a ping message to the client
func (c *Connection) Ping(writeWait time.Duration) {
	c.conn.SetWriteDeadline(time.Now().Add(writeWait))
	if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
		return
	}
}

// ServeWs handles websocket requests from the peer.
func ServeWs(hub *Hub, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(err)
		return
	}
}
