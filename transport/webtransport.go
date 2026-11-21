package transport

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/webtransport-go"
)

// messages of a client that wait to be read by the game
const sessionInbox = 256

var errMessageTooLong = errors.New("message is longer than allowed")

// sessionConnection is a client connected over WebTransport.
//
// The reliable lane is one stream in both directions that the client opens;
// a message on it is its length (two bytes, little endian) and its bytes.
// The unreliable lane are datagrams, one message each.
type sessionConnection struct {
	session *webtransport.Session
	// messages of both lanes in the order they arrived
	inbox chan []byte

	// the stream, once the client has opened it
	stream      *webtransport.Stream
	streamReady chan struct{}
	writeMutex  sync.Mutex
	writeWait   time.Duration

	// set by the goroutine that reads messages, used by the one that reads the stream
	maxMessageSize atomic.Int64
	closeOnce      sync.Once
}

func newSessionConnection(session *webtransport.Session) *sessionConnection {
	c := &sessionConnection{
		session:     session,
		inbox:       make(chan []byte, sessionInbox),
		streamReady: make(chan struct{}),
		writeWait:   time.Second,
	}
	c.maxMessageSize.Store(1024)
	go c.readDatagrams()
	go c.readStream()
	return c
}

// Identifier of the connection
func (c *sessionConnection) Identifier() string {
	return fmt.Sprintf("webtransport - %s", c.session.RemoteAddr())
}

func (c *sessionConnection) readDatagrams() {
	for {
		data, err := c.session.ReceiveDatagram(c.session.Context())
		if err != nil {
			return
		}
		// what does not fit is lost, like any datagram can be
		select {
		case c.inbox <- data:
		default:
		}
	}
}

func (c *sessionConnection) readStream() {
	stream, err := c.session.AcceptStream(c.session.Context())
	if err != nil {
		return
	}
	c.stream = stream
	close(c.streamReady)
	header := make([]byte, 2)
	for {
		if _, err := io.ReadFull(stream, header); err != nil {
			c.fail(err)
			return
		}
		length := int64(binary.LittleEndian.Uint16(header))
		if length > c.maxMessageSize.Load() {
			c.fail(errMessageTooLong)
			return
		}
		data := make([]byte, length)
		if _, err := io.ReadFull(stream, data); err != nil {
			c.fail(err)
			return
		}
		select {
		case c.inbox <- data:
		case <-c.session.Context().Done():
			return
		}
	}
}

// fail ends the session after the reliable lane broke
func (c *sessionConnection) fail(err error) {
	c.closeOnce.Do(func() {
		c.session.CloseWithError(1, err.Error())
	})
}

// PrepareRead sets the longest message a client may send. A client that is
// gone is noticed by the session itself (see sessionIdleTimeout).
func (c *sessionConnection) PrepareRead(maxMessageSize int64, pongWait time.Duration) {
	c.maxMessageSize.Store(maxMessageSize)
}

// Read returns the next message of either lane
func (c *sessionConnection) Read() ([]byte, error) {
	select {
	case data := <-c.inbox:
		return data, nil
	case <-c.session.Context().Done():
		return nil, context.Cause(c.session.Context())
	}
}

// PrepareWrite sets how long a write to the stream may take
func (c *sessionConnection) PrepareWrite(writeWait time.Duration) {
	c.writeWait = writeWait
}

// Write sends a message on the reliable lane
func (c *sessionConnection) Write(data []byte) error {
	if len(data) > 0xffff {
		return errMessageTooLong
	}
	// the client opens the stream; it has, if it said hello
	select {
	case <-c.streamReady:
	default:
		select {
		case <-c.streamReady:
		case <-c.session.Context().Done():
			return context.Cause(c.session.Context())
		case <-time.After(c.writeWait):
			return errors.New("the client has not opened the reliable stream")
		}
	}
	framed := make([]byte, 2+len(data))
	binary.LittleEndian.PutUint16(framed, uint16(len(data)))
	copy(framed[2:], data)
	c.writeMutex.Lock()
	defer c.writeMutex.Unlock()
	c.stream.SetWriteDeadline(time.Now().Add(c.writeWait))
	_, err := c.stream.Write(framed)
	return err
}

// WriteUnreliable sends a message as a datagram. One that can not be sent
// now is lost; the next snapshot replaces it. One that is too large for a
// packet would be lost every time: it goes over the stream instead.
func (c *sessionConnection) WriteUnreliable(data []byte) error {
	err := c.session.SendDatagram(data)
	if err == nil {
		return nil
	}
	var tooLarge *quic.DatagramTooLargeError
	if errors.As(err, &tooLarge) {
		return c.Write(data)
	}
	// only a session that is gone is an error for the caller
	if c.session.Context().Err() != nil {
		return context.Cause(c.session.Context())
	}
	return nil
}

// Ping does nothing, the session keeps itself alive
func (c *sessionConnection) Ping(writeWait time.Duration) {}

// Close ends the session
func (c *sessionConnection) Close(writeWait time.Duration, graceful bool) {
	c.closeOnce.Do(func() {
		c.session.CloseWithError(0, "")
	})
}
