package transport

import (
	"fmt"
	"time"

	"github.com/gorilla/websocket"
)

// Connection represents a websocket connection
type Connection struct {
	conn     *websocket.Conn
	pongWait time.Duration
}

// NewConnection creates a new connection
func NewConnection(conn *websocket.Conn) *Connection {
	return &Connection{
		conn: conn,
	}
}

// Identifier of the connection
func (c *Connection) Identifier() string {
	return fmt.Sprintf("%s - %s", c.conn.RemoteAddr().Network(), c.conn.RemoteAddr().String())
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
	c.pongWait = pongWait
	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error { c.conn.SetReadDeadline(time.Now().Add(pongWait)); return nil })
}

// Read from the network connection
func (c *Connection) Read() ([]byte, error) {
	_, message, err := c.conn.ReadMessage()
	if err != nil {
		if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
			// TODO: wrap in another error
		}
		return nil, err
	}
	// any message proves the client is alive, not only pongs
	c.conn.SetReadDeadline(time.Now().Add(c.pongWait))
	return message, nil
}

// PrepareWrite prepares the websocket connection for writing
func (c *Connection) PrepareWrite(writeWait time.Duration) {
	c.conn.SetWriteDeadline(time.Now().Add(writeWait))
}

// Write to the network connection
func (c *Connection) Write(data []byte) error {
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

// WriteUnreliable sends on the same socket: a websocket has one lane
func (c *Connection) WriteUnreliable(data []byte) error {
	return c.Write(data)
}

// Ping sends a ping message to the client
func (c *Connection) Ping(writeWait time.Duration) {
	c.conn.SetWriteDeadline(time.Now().Add(writeWait))
	if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
		return
	}
}
