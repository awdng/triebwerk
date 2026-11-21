// Package transport accepts the connections of clients and hands them to
// the game as model.Connection, whatever they are made of:
//
// A WebSocket at /echo, always. Both lanes of the protocol share the
// socket, so everything arrives in order and a lost packet holds up what
// was sent after it.
//
// WebTransport at /wt on the same port number (UDP), when there is a
// certificate. The reliable lane is a stream, the unreliable lane are
// datagrams: a lost snapshot does not delay the next one.
//
// GET /transport says which of the two a server offers.
package transport

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/awdng/triebwerk/model"
	"github.com/gorilla/websocket"
	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"github.com/quic-go/webtransport-go"
)

const (
	webSocketPath    = "/echo"
	webTransportPath = "/wt"
	offerPath        = "/transport"
	// a client that is not heard from for this long is gone
	sessionIdleTimeout = 6 * time.Second
	sessionKeepAlive   = 2 * time.Second
)

// Config of a transport
type Config struct {
	// Address clients reach the server at, reported to the master server
	Address string
	// Port for the WebSocket (TCP) and for WebTransport (UDP), 0 picks a free one
	Port int
	// CertFile and KeyFile of a certificate for Address. With one the
	// WebSocket is served over TLS (wss) and WebTransport is offered.
	CertFile string
	KeyFile  string
	// SelfSigned offers WebTransport without a certificate file: the server
	// makes one that is valid for two weeks and tells clients its hash, by
	// which browsers accept it (Chromium does). The WebSocket stays without
	// TLS, browsers take no hash for it. For development and tests.
	SelfSigned bool
}

// Transport listens for clients
type Transport struct {
	config     Config
	upgrader   websocket.Upgrader
	register   func(conn model.Connection)
	unregister func(conn model.Connection)
	mux        *http.ServeMux

	mutex     sync.RWMutex
	port      int
	tcp       *http.Server
	sessions  *webtransport.Server
	ephemeral *ephemeralCertificate
	ready     bool
}

// New creates a transport, nothing listens before Run
func New(config Config) *Transport {
	return &Transport{
		config: config,
		port:   config.Port,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
		register:   func(model.Connection) {},
		unregister: func(model.Connection) {},
	}
}

// GetAddress ...
func (t *Transport) GetAddress() string {
	t.mutex.RLock()
	defer t.mutex.RUnlock()
	return net.JoinHostPort(t.config.Address, strconv.Itoa(t.port))
}

// RegisterNewConnHandler is a callback for new connections
func (t *Transport) RegisterNewConnHandler(register func(conn model.Connection)) {
	t.register = register
}

// UnregisterConnHandler is a callback for closed connections
func (t *Transport) UnregisterConnHandler(unregister func(conn model.Connection)) {
	t.unregister = unregister
}

// Unregister callback
func (t *Transport) Unregister(conn model.Connection) {
	t.unregister(conn)
}

func (t *Transport) offersWebTransport() bool {
	return t.config.SelfSigned || t.config.CertFile != ""
}

// Init ...
func (t *Transport) Init() {
	t.mux = http.NewServeMux()
	t.mux.HandleFunc(webSocketPath, func(w http.ResponseWriter, r *http.Request) {
		ws, err := t.upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Println(err)
			return
		}
		t.register(NewConnection(ws))
	})
	t.mux.HandleFunc(offerPath, t.serveOffer)
}

// what GET /transport answers
type offer struct {
	WebTransport *webTransportOffer `json:"webtransport,omitempty"`
}

type webTransportOffer struct {
	// on the port the question was asked on
	Path string `json:"path"`
	// base64 of the sha256 of a certificate that no authority signed; a
	// browser is told to accept the server by it
	CertHash string `json:"certHash,omitempty"`
}

func (t *Transport) serveOffer(w http.ResponseWriter, r *http.Request) {
	var o offer
	if t.offersWebTransport() {
		o.WebTransport = &webTransportOffer{Path: webTransportPath}
		if t.ephemeral != nil {
			o.WebTransport.CertHash = t.ephemeral.hash(time.Now())
		}
	}
	// the page that asks comes from another origin than the game server
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(o)
}

// tlsConfig returns the certificate WebTransport is served with, nil if none is offered
func (t *Transport) tlsConfig() (*tls.Config, error) {
	switch {
	case t.config.CertFile != "":
		cert, err := tls.LoadX509KeyPair(t.config.CertFile, t.config.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("can not load the certificate %s: %w", t.config.CertFile, err)
		}
		return &tls.Config{Certificates: []tls.Certificate{cert}}, nil
	case t.config.SelfSigned:
		t.ephemeral = newEphemeralCertificate([]string{"localhost", "127.0.0.1", "::1", t.config.Address})
		return &tls.Config{
			GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
				return t.ephemeral.certificate(time.Now())
			},
		}, nil
	}
	return nil, nil
}

// Run listens and serves until the transport is closed
func (t *Transport) Run() error {
	tlsConf, err := t.tlsConfig()
	if err != nil {
		return err
	}
	var listener net.Listener
	var udp net.PacketConn
	if tlsConf != nil {
		listener, udp, err = listenBoth(t.config.Port, net.Listen, net.ListenPacket)
	} else {
		listener, err = net.Listen("tcp", fmt.Sprintf(":%d", t.config.Port))
	}
	if err != nil {
		return err
	}
	port := listener.Addr().(*net.TCPAddr).Port
	tcp := &http.Server{Handler: t.mux}

	var sessions *webtransport.Server
	if udp != nil {
		sessions = t.serveSessions(udp, tlsConf)
	}

	t.mutex.Lock()
	t.port = port
	t.tcp = tcp
	t.sessions = sessions
	t.ready = true
	t.mutex.Unlock()

	if t.config.CertFile != "" {
		log.Printf("Starting Triebwerk Server on %s: WebSocket over TLS and WebTransport ...", t.GetAddress())
		return tcp.ServeTLS(listener, t.config.CertFile, t.config.KeyFile)
	}
	if sessions != nil {
		log.Printf("Starting Triebwerk Server on %s: WebSocket and WebTransport with a certificate of its own ...", t.GetAddress())
	} else {
		log.Printf("Starting Triebwerk Websocket Server on %s...", t.GetAddress())
	}
	return tcp.Serve(listener)
}

// listenBoth listens on the same port number for TCP (the websocket) and
// UDP (WebTransport). With port 0 the system picks the TCP port; if that
// number is taken on UDP another one is picked.
func listenBoth(port int, listenTCP func(network, address string) (net.Listener, error), listenUDP func(network, address string) (net.PacketConn, error)) (net.Listener, net.PacketConn, error) {
	const attempts = 20
	for attempt := 1; ; attempt++ {
		listener, err := listenTCP("tcp", fmt.Sprintf(":%d", port))
		if err != nil {
			return nil, nil, err
		}
		picked := listener.Addr().(*net.TCPAddr).Port
		udp, err := listenUDP("udp", fmt.Sprintf(":%d", picked))
		if err == nil {
			return listener, udp, nil
		}
		listener.Close()
		if port != 0 || attempt == attempts {
			return nil, nil, fmt.Errorf("WebTransport needs UDP port %d: %w", picked, err)
		}
	}
}

// serveSessions accepts WebTransport sessions on a UDP socket
func (t *Transport) serveSessions(udp net.PacketConn, tlsConf *tls.Config) *webtransport.Server {
	mux := http.NewServeMux()
	h3 := &http3.Server{
		TLSConfig: http3.ConfigureTLSConfig(tlsConf),
		QUICConfig: &quic.Config{
			EnableDatagrams: true,
			// what WebTransport streams need to be reset cleanly
			EnableStreamResetPartialDelivery: true,
			MaxIdleTimeout:                   sessionIdleTimeout,
			KeepAlivePeriod:                  sessionKeepAlive,
		},
		Handler: mux,
	}
	webtransport.ConfigureHTTP3Server(h3)
	server := &webtransport.Server{
		H3: h3,
		// pages of any origin may play, as with the websocket
		CheckOrigin: func(*http.Request) bool { return true },
	}
	mux.HandleFunc(webTransportPath, func(w http.ResponseWriter, r *http.Request) {
		session, err := server.Upgrade(w, r)
		if err != nil {
			log.Printf("WebTransport: %s", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		t.register(newSessionConnection(session))
	})
	// a browser that already talks HTTP/3 to this server asks here
	mux.HandleFunc(offerPath, t.serveOffer)
	go func() {
		if err := server.Serve(udp); err != nil && err != http.ErrServerClosed {
			log.Printf("WebTransport: %s", err)
		}
	}()
	return server
}

// listening is true once clients can connect
func (t *Transport) listening() bool {
	t.mutex.RLock()
	defer t.mutex.RUnlock()
	return t.ready
}

// Close stops listening and drops every client
func (t *Transport) Close() {
	t.mutex.Lock()
	tcp, sessions := t.tcp, t.sessions
	t.tcp, t.sessions = nil, nil
	t.mutex.Unlock()
	if tcp != nil {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		tcp.Shutdown(ctx)
		tcp.Close()
	}
	if sessions != nil {
		sessions.Close()
	}
}
