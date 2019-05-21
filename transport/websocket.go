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

// PrepareRead prepares the websocket connection for reading
func (c *Connection) PrepareRead(maxMessageSize int64, pongWait time.Duration) {
	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error { c.conn.SetReadDeadline(time.Now().Add(pongWait)); return nil })
}

// Read from the network connection
func (c *Connection) Read() ([]byte, error) {
	messageType, message, err := c.conn.ReadMessage()
	if err != nil {
		if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
			// TODO: wrap in another error
		}
		return nil, err
	}
	return message, nil
}

// PrepareWrite prepares the websocket connection for writing
func (c *Connection) PrepareWrite(writeWait time.Duration) {
	c.conn.SetWriteDeadline(time.Now().Add(writeWait))
}

// Write to the network connection
func (c *Connection) Write(data []byte) error {
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
