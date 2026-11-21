package loadtest

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/awdng/triebwerk/protocol"
	"github.com/gorilla/websocket"
	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"github.com/quic-go/webtransport-go"
)

// conn is a bot's connection to the server, made of whatever the address says
type conn interface {
	// Send puts a message on the lane its type travels on
	Send(data []byte) error
	// Read returns the next message of either lane
	Read(until time.Time) ([]byte, error)
	Close()
}

func dial(url string) (conn, error) {
	if strings.HasPrefix(url, "https://") {
		return dialWebTransport(url)
	}
	return dialWebSocket(url)
}

// ---- websocket: one socket for both lanes ----------------------------------------

type webSocketConn struct {
	socket *websocket.Conn
	mutex  sync.Mutex
}

func dialWebSocket(url string) (conn, error) {
	// wss: the certificate is not checked, like for WebTransport
	dialer := websocket.Dialer{
		HandshakeTimeout: 5 * time.Second,
		TLSClientConfig:  &tls.Config{InsecureSkipVerify: true},
	}
	socket, _, err := dialer.Dial(url, nil)
	if err != nil {
		return nil, err
	}
	return &webSocketConn{socket: socket}, nil
}

func (c *webSocketConn) Send(data []byte) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.socket.SetWriteDeadline(time.Now().Add(2 * time.Second))
	return c.socket.WriteMessage(websocket.BinaryMessage, data)
}

func (c *webSocketConn) Read(until time.Time) ([]byte, error) {
	c.socket.SetReadDeadline(until)
	_, data, err := c.socket.ReadMessage()
	return data, err
}

func (c *webSocketConn) Close() {
	c.socket.Close()
}

// ---- WebTransport: a stream for the reliable lane, datagrams for the other --------

type webTransportConn struct {
	transport *webtransport.Transport
	session   *webtransport.Session
	stream    *webtransport.Stream
	mutex     sync.Mutex
	inbox     chan []byte
}

func dialWebTransport(url string) (conn, error) {
	transport := &webtransport.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true, NextProtos: []string{http3.NextProtoH3}},
		QUICConfig: &quic.Config{
			EnableDatagrams:                  true,
			EnableStreamResetPartialDelivery: true,
			KeepAlivePeriod:                  2 * time.Second,
			// fits through a VPN tunnel, see the server's transport package
			InitialPacketSize: 1200,
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, session, err := transport.Dial(ctx, url, nil)
	if err != nil {
		transport.Close()
		return nil, err
	}
	stream, err := session.OpenStreamSync(ctx)
	if err != nil {
		session.CloseWithError(0, "")
		transport.Close()
		return nil, err
	}
	c := &webTransportConn{transport: transport, session: session, stream: stream, inbox: make(chan []byte, 256)}
	go c.readDatagrams()
	go c.readStream()
	return c, nil
}

func (c *webTransportConn) readDatagrams() {
	for {
		data, err := c.session.ReceiveDatagram(c.session.Context())
		if err != nil {
			return
		}
		select {
		case c.inbox <- data:
		default: // a bot that does not keep up loses snapshots, like a browser would
		}
	}
}

// messages on the stream are their length (two bytes, little endian) and their bytes
func (c *webTransportConn) readStream() {
	header := make([]byte, 2)
	for {
		if _, err := io.ReadFull(c.stream, header); err != nil {
			c.session.CloseWithError(0, "")
			return
		}
		data := make([]byte, binary.LittleEndian.Uint16(header))
		if _, err := io.ReadFull(c.stream, data); err != nil {
			c.session.CloseWithError(0, "")
			return
		}
		select {
		case c.inbox <- data:
		case <-c.session.Context().Done():
			return
		}
	}
}

func (c *webTransportConn) Send(data []byte) error {
	if len(data) > 0 && !protocol.IsReliable(data[0]) {
		if err := c.session.SendDatagram(data); err != nil && c.session.Context().Err() != nil {
			return err
		}
		return nil
	}
	framed := append(binary.LittleEndian.AppendUint16(make([]byte, 0, 2+len(data)), uint16(len(data))), data...)
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.stream.SetWriteDeadline(time.Now().Add(2 * time.Second))
	_, err := c.stream.Write(framed)
	return err
}

func (c *webTransportConn) Read(until time.Time) ([]byte, error) {
	timer := time.NewTimer(time.Until(until))
	defer timer.Stop()
	select {
	case data := <-c.inbox:
		return data, nil
	case <-c.session.Context().Done():
		return nil, context.Cause(c.session.Context())
	case <-timer.C:
		return nil, errors.New("deadline reached")
	}
}

func (c *webTransportConn) Close() {
	c.session.CloseWithError(0, "")
	c.transport.Close()
}
