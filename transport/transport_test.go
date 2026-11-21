package transport

import (
	"context"
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/awdng/triebwerk/model"
	gorilla "github.com/gorilla/websocket"
	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"github.com/quic-go/webtransport-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// a running transport and the connections that arrive at it
type testServer struct {
	t         *testing.T
	transport *Transport
	conns     chan model.Connection
	gone      chan model.Connection
	port      string
}

func start(t *testing.T, config Config) *testServer {
	s := &testServer{t: t, transport: New(config), conns: make(chan model.Connection, 8), gone: make(chan model.Connection, 8)}
	s.transport.RegisterNewConnHandler(func(conn model.Connection) { s.conns <- conn })
	s.transport.UnregisterConnHandler(func(conn model.Connection) { s.gone <- conn })
	s.transport.Init()
	go s.transport.Run()
	t.Cleanup(s.transport.Close)
	require.Eventually(t, func() bool {
		_, port, err := net.SplitHostPort(s.transport.GetAddress())
		s.port = port
		return err == nil && port != "0" && s.transport.listening()
	}, 5*time.Second, 10*time.Millisecond)
	return s
}

func (s *testServer) accept() model.Connection {
	s.t.Helper()
	select {
	case conn := <-s.conns:
		conn.PrepareRead(1024, 5*time.Second)
		return conn
	case <-time.After(5 * time.Second):
		s.t.Fatal("no connection arrived")
		return nil
	}
}

var insecure = &tls.Config{InsecureSkipVerify: true}

// ---- websocket -------------------------------------------------------------------

func TestWebSocketCarriesBothLanesOnOneSocket(t *testing.T) {
	s := start(t, Config{Address: "localhost"})
	client, _, err := gorilla.DefaultDialer.Dial("ws://localhost:"+s.port+"/echo", nil)
	require.NoError(t, err)
	defer client.Close()
	conn := s.accept()
	assert.Contains(t, conn.Identifier(), "tcp")

	require.NoError(t, client.WriteMessage(gorilla.BinaryMessage, []byte{1, 2, 3}))
	data, err := conn.Read()
	require.NoError(t, err)
	assert.Equal(t, []byte{1, 2, 3}, data)

	conn.PrepareWrite(time.Second)
	require.NoError(t, conn.Write([]byte{2, 9}))
	require.NoError(t, conn.WriteUnreliable([]byte{17, 8}))
	_, first, err := client.ReadMessage()
	require.NoError(t, err)
	_, second, err := client.ReadMessage()
	require.NoError(t, err)
	assert.Equal(t, [][]byte{{2, 9}, {17, 8}}, [][]byte{first, second})
}

// what a client reads from GET /transport
type clientOffer struct {
	WebTransport *struct {
		Path     string `json:"path"`
		CertHash string `json:"certHash"`
	} `json:"webtransport"`
}

func fetchOffer(t *testing.T, client *http.Client, url string) (clientOffer, http.Header) {
	t.Helper()
	response, err := client.Get(url)
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, http.StatusOK, response.StatusCode)
	var o clientOffer
	require.NoError(t, json.NewDecoder(response.Body).Decode(&o))
	return o, response.Header
}

func TestServerWithoutCertificateOffersOnlyTheWebSocket(t *testing.T) {
	s := start(t, Config{Address: "localhost"})
	o, header := fetchOffer(t, http.DefaultClient, "http://localhost:"+s.port+"/transport")
	assert.Nil(t, o.WebTransport)
	// pages of any origin ask
	assert.Equal(t, "*", header.Get("Access-Control-Allow-Origin"))
}

// ---- webtransport ----------------------------------------------------------------

// dial opens a session and the stream that is the reliable lane
func dial(t *testing.T, port string) (*webtransport.Session, *webtransport.Stream) {
	t.Helper()
	d := &webtransport.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true, NextProtos: []string{http3.NextProtoH3}},
		QUICConfig:      &quic.Config{EnableDatagrams: true, EnableStreamResetPartialDelivery: true},
	}
	t.Cleanup(func() { d.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, session, err := d.Dial(ctx, "https://localhost:"+port+"/wt", nil)
	require.NoError(t, err)
	t.Cleanup(func() { session.CloseWithError(0, "") })
	stream, err := session.OpenStreamSync(ctx)
	require.NoError(t, err)
	return session, stream
}

func frame(data []byte) []byte {
	return append(binary.LittleEndian.AppendUint16(nil, uint16(len(data))), data...)
}

func readFrame(t *testing.T, stream *webtransport.Stream) []byte {
	t.Helper()
	stream.SetReadDeadline(time.Now().Add(5 * time.Second))
	header := make([]byte, 2)
	_, err := io.ReadFull(stream, header)
	require.NoError(t, err)
	data := make([]byte, binary.LittleEndian.Uint16(header))
	_, err = io.ReadFull(stream, data)
	require.NoError(t, err)
	return data
}

func TestSelfSignedServerOffersWebTransportWithTheHashOfItsCertificate(t *testing.T) {
	s := start(t, Config{Address: "localhost", SelfSigned: true})
	o, _ := fetchOffer(t, http.DefaultClient, "http://localhost:"+s.port+"/transport")
	require.NotNil(t, o.WebTransport)
	assert.Equal(t, "/wt", o.WebTransport.Path)

	// what a browser checks: the hash, an ECDSA key, at most 14 days of validity
	conn, err := quic.DialAddr(context.Background(), "localhost:"+s.port, &tls.Config{InsecureSkipVerify: true, NextProtos: []string{http3.NextProtoH3}}, nil)
	require.NoError(t, err)
	defer conn.CloseWithError(0, "")
	cert := conn.ConnectionState().TLS.PeerCertificates[0]
	hash := sha256.Sum256(cert.Raw)
	assert.Equal(t, base64.StdEncoding.EncodeToString(hash[:]), o.WebTransport.CertHash)
	_, isECDSA := cert.PublicKey.(*ecdsa.PublicKey)
	assert.True(t, isECDSA)
	assert.LessOrEqual(t, cert.NotAfter.Sub(cert.NotBefore), 14*24*time.Hour)
	assert.True(t, cert.NotBefore.Before(time.Now()))
	assert.NoError(t, cert.VerifyHostname("localhost"))

	// the websocket stays as it is: browsers take no hash for it
	client, _, err := gorilla.DefaultDialer.Dial("ws://localhost:"+s.port+"/echo", nil)
	require.NoError(t, err)
	client.Close()
}

func TestWebTransportCarriesTheLanesSeparately(t *testing.T) {
	s := start(t, Config{Address: "localhost", SelfSigned: true})
	session, stream := dial(t, s.port)
	conn := s.accept()
	assert.Contains(t, conn.Identifier(), "webtransport")

	// client to server: the stream carries messages with their length, datagrams are messages
	_, err := stream.Write(append(frame([]byte{1, 2, 'a'}), frame([]byte{1, 2, 'b', 'c'})...))
	require.NoError(t, err)
	first, err := conn.Read()
	require.NoError(t, err)
	second, err := conn.Read()
	require.NoError(t, err)
	assert.Equal(t, [][]byte{{1, 2, 'a'}, {1, 2, 'b', 'c'}}, [][]byte{first, second})
	require.NoError(t, session.SendDatagram([]byte{16, 1, 0, 0, 0, 1, 0}))
	datagram, err := conn.Read()
	require.NoError(t, err)
	assert.Equal(t, []byte{16, 1, 0, 0, 0, 1, 0}, datagram)

	// server to client
	conn.PrepareWrite(time.Second)
	require.NoError(t, conn.Write([]byte{2, 2, 7}))
	require.NoError(t, conn.Write([]byte{4, 1, 0, 0, 0}))
	require.NoError(t, conn.WriteUnreliable([]byte{17, 5}))
	assert.Equal(t, []byte{2, 2, 7}, readFrame(t, stream))
	assert.Equal(t, []byte{4, 1, 0, 0, 0}, readFrame(t, stream))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	received, err := session.ReceiveDatagram(ctx)
	require.NoError(t, err)
	assert.Equal(t, []byte{17, 5}, received)
}

func TestWebTransportMessageInPiecesIsOneMessage(t *testing.T) {
	s := start(t, Config{Address: "localhost", SelfSigned: true})
	_, stream := dial(t, s.port)
	conn := s.accept()
	whole := frame([]byte{1, 2, 'l', 'o', 'n', 'g'})
	for _, piece := range [][]byte{whole[:1], whole[1:4], whole[4:]} {
		_, err := stream.Write(piece)
		require.NoError(t, err)
		time.Sleep(20 * time.Millisecond)
	}
	data, err := conn.Read()
	require.NoError(t, err)
	assert.Equal(t, []byte{1, 2, 'l', 'o', 'n', 'g'}, data)
}

func TestWebTransportReadEndsWhenTheClientLeaves(t *testing.T) {
	s := start(t, Config{Address: "localhost", SelfSigned: true})
	session, _ := dial(t, s.port)
	conn := s.accept()
	done := make(chan error, 1)
	go func() {
		_, err := conn.Read()
		done <- err
	}()
	session.CloseWithError(0, "bye")
	select {
	case err := <-done:
		assert.Error(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("the server still waits for a client that left")
	}
	// writing to it fails instead of blocking
	conn.PrepareWrite(100 * time.Millisecond)
	assert.Error(t, conn.Write([]byte{2}))
}

func TestWebTransportClosesAClientWhoseMessageIsTooLong(t *testing.T) {
	s := start(t, Config{Address: "localhost", SelfSigned: true})
	_, stream := dial(t, s.port)
	conn := s.accept() // reads at most 1024 bytes per message
	_, err := stream.Write(frame(make([]byte, 5000)))
	require.NoError(t, err)
	_, err = conn.Read()
	assert.Error(t, err)
}

func TestWebTransportClosedByTheServerIsClosedForTheClient(t *testing.T) {
	s := start(t, Config{Address: "localhost", SelfSigned: true})
	session, _ := dial(t, s.port)
	conn := s.accept()
	conn.Close(time.Second, false)
	select {
	case <-session.Context().Done():
	case <-time.After(5 * time.Second):
		t.Fatal("the client was not told")
	}
}

// ---- a certificate from files: what a public server runs with --------------------------------

func writeCertificate(t *testing.T) (certFile, keyFile string) {
	t.Helper()
	cert, err := selfSignedCertificate([]string{"localhost"}, time.Now())
	require.NoError(t, err)
	dir := t.TempDir()
	certFile, keyFile = filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
	require.NoError(t, os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Certificate[0]}), 0o600))
	key, err := x509.MarshalPKCS8PrivateKey(cert.PrivateKey)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: key}), 0o600))
	return certFile, keyFile
}

func TestServerWithACertificateServesEverythingOverTLS(t *testing.T) {
	certFile, keyFile := writeCertificate(t)
	s := start(t, Config{Address: "localhost", CertFile: certFile, KeyFile: keyFile})

	// the websocket, encrypted
	dialer := gorilla.Dialer{TLSClientConfig: insecure}
	client, _, err := dialer.Dial("wss://localhost:"+s.port+"/echo", nil)
	require.NoError(t, err)
	defer client.Close()
	conn := s.accept()
	require.NoError(t, client.WriteMessage(gorilla.BinaryMessage, []byte{1, 2}))
	data, err := conn.Read()
	require.NoError(t, err)
	assert.Equal(t, []byte{1, 2}, data)
	_, _, err = gorilla.DefaultDialer.Dial("ws://localhost:"+s.port+"/echo", nil)
	assert.Error(t, err, "no websocket without TLS next to it")

	// WebTransport on the same port number; the certificate speaks for itself, no hash
	o, _ := fetchOffer(t, &http.Client{Transport: &http.Transport{TLSClientConfig: insecure}}, "https://localhost:"+s.port+"/transport")
	require.NotNil(t, o.WebTransport)
	assert.Equal(t, "/wt", o.WebTransport.Path)
	assert.Empty(t, o.WebTransport.CertHash)
	_, stream := dial(t, s.port)
	second := s.accept()
	_, err = stream.Write(frame([]byte{1, 2, 'x'}))
	require.NoError(t, err)
	data, err = second.Read()
	require.NoError(t, err)
	assert.Equal(t, []byte{1, 2, 'x'}, data)
}

func TestServerDoesNotStartWithACertificateItCanNotRead(t *testing.T) {
	transport := New(Config{Address: "localhost", CertFile: "/nonexistent/cert.pem", KeyFile: "/nonexistent/key.pem"})
	transport.Init()
	err := transport.Run()
	assert.Error(t, err)
	assert.Contains(t, fmt.Sprint(err), "certificate")
}

// With port 0 the system picks the TCP port, and WebTransport wants the same
// number on UDP. That one can be taken, by anybody's outgoing socket.
func TestPickedPortThatIsTakenOnUDPIsPickedAgain(t *testing.T) {
	attempts := 0
	listenUDP := func(network, address string) (net.PacketConn, error) {
		attempts++
		if attempts < 3 {
			return nil, fmt.Errorf("listen udp %s: address already in use", address)
		}
		return net.ListenPacket(network, address)
	}
	listener, udp, err := listenBoth(0, net.Listen, listenUDP)
	require.NoError(t, err)
	defer listener.Close()
	defer udp.Close()
	assert.Equal(t, 3, attempts)
	assert.Equal(t, listener.Addr().(*net.TCPAddr).Port, udp.LocalAddr().(*net.UDPAddr).Port)
}

// a port somebody asked for is that port or an error
func TestConfiguredPortThatIsTakenOnUDPIsAnError(t *testing.T) {
	taken, err := net.ListenPacket("udp", ":0")
	require.NoError(t, err)
	defer taken.Close()
	port := taken.LocalAddr().(*net.UDPAddr).Port
	_, _, err = listenBoth(port, net.Listen, net.ListenPacket)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "WebTransport needs UDP port")
	// and the TCP port is free again
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	require.NoError(t, err)
	listener.Close()
}

// A datagram is one packet. What does not fit into one still has to arrive:
// a snapshot of a very full server goes over the stream instead.
func TestWebTransportSendsWhatIsTooLargeForADatagramOnTheStream(t *testing.T) {
	s := start(t, Config{Address: "localhost", SelfSigned: true})
	_, stream := dial(t, s.port)
	conn := s.accept()
	// the client has opened its stream when its first message is there
	_, err := stream.Write(frame([]byte{1, 2}))
	require.NoError(t, err)
	_, err = conn.Read()
	require.NoError(t, err)

	large := make([]byte, 3000)
	large[0] = 17
	conn.PrepareWrite(time.Second)
	require.NoError(t, conn.WriteUnreliable(large))
	assert.Equal(t, large, readFrame(t, stream))
}

// A browser that already talks HTTP/3 to the server asks there as well.
func TestOfferIsAnsweredOverHTTP3Too(t *testing.T) {
	s := start(t, Config{Address: "localhost", SelfSigned: true})
	roundTripper := &http3.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true, NextProtos: []string{http3.NextProtoH3}}}
	defer roundTripper.Close()
	o, header := fetchOffer(t, &http.Client{Transport: roundTripper, Timeout: 5 * time.Second}, "https://localhost:"+s.port+"/transport")
	require.NotNil(t, o.WebTransport)
	assert.Equal(t, "/wt", o.WebTransport.Path)
	assert.NotEmpty(t, o.WebTransport.CertHash)
	assert.Equal(t, "*", header.Get("Access-Control-Allow-Origin"))
}

// narrowPath relays UDP datagrams between a client and the server like a
// path whose packets may be at most mtu bytes long: larger ones vanish.
// Returns the port clients connect to.
func narrowPath(t *testing.T, serverPort string, mtu int) string {
	t.Helper()
	// what is left for the payload of a datagram: 20 bytes IPv4, 8 bytes UDP
	limit := mtu - 28
	front, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { front.Close() })
	server, err := net.ResolveUDPAddr("udp", "127.0.0.1:"+serverPort)
	require.NoError(t, err)

	go func() {
		backs := map[string]*net.UDPConn{}
		defer func() {
			for _, back := range backs {
				back.Close()
			}
		}()
		buf := make([]byte, 65536)
		for {
			n, client, err := front.ReadFrom(buf)
			if err != nil {
				return
			}
			if n > limit {
				continue
			}
			back := backs[client.String()]
			if back == nil {
				back, err = net.DialUDP("udp", nil, server)
				if err != nil {
					return
				}
				backs[client.String()] = back
				go func(back *net.UDPConn, client net.Addr) {
					reply := make([]byte, 65536)
					for {
						n, err := back.Read(reply)
						if err != nil {
							return
						}
						if n <= limit {
							front.WriteTo(reply[:n], client)
						}
					}
				}(back, client)
			}
			back.Write(buf[:n])
		}
	}()
	_, port, err := net.SplitHostPort(front.LocalAddr().String())
	require.NoError(t, err)
	return port
}

// A VPN tunnel carries packets of 1280 bytes and no more (Tailscale,
// WireGuard). Every QUIC path has to carry 1200 bytes of payload, which
// fits; the library's own first guess of 1280 bytes of payload does not,
// and the first packet of the server's handshake is always full.
func TestWebTransportConnectsOverAPathThatCarriesOnly1280Bytes(t *testing.T) {
	s := start(t, Config{Address: "localhost", SelfSigned: true})
	port := narrowPath(t, s.port, 1280)

	d := &webtransport.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true, NextProtos: []string{http3.NextProtoH3}},
		// a browser sends packets that fit
		QUICConfig: &quic.Config{EnableDatagrams: true, EnableStreamResetPartialDelivery: true, InitialPacketSize: 1200},
	}
	defer d.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, session, err := d.Dial(ctx, "https://127.0.0.1:"+port+"/wt", nil)
	require.NoError(t, err, "the handshake did not get through")
	defer session.CloseWithError(0, "")

	// and it plays: a stream message, a datagram each way, a full snapshot
	stream, err := session.OpenStreamSync(ctx)
	require.NoError(t, err)
	conn := s.accept()
	_, err = stream.Write(frame([]byte{1, 2, 'x'}))
	require.NoError(t, err)
	data, err := conn.Read()
	require.NoError(t, err)
	assert.Equal(t, []byte{1, 2, 'x'}, data)

	snapshot := make([]byte, 1100)
	snapshot[0] = 17
	require.NoError(t, conn.WriteUnreliable(snapshot))
	received, err := session.ReceiveDatagram(ctx)
	require.NoError(t, err)
	assert.Equal(t, snapshot, received)
}

// the game server serves more than the game on its port: the live view
func TestTransportServesWhatItWasHandedNextToTheGame(t *testing.T) {
	tr := New(Config{Address: "localhost"})
	tr.Handle("/spectate/", http.StripPrefix("/spectate", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("view " + r.URL.Path))
	})))
	tr.Init()

	recorder := httptest.NewRecorder()
	tr.mux.ServeHTTP(recorder, httptest.NewRequest("GET", "/spectate/map.json", nil))
	assert.Equal(t, "view /map.json", recorder.Body.String())

	// the game's own paths are still there
	recorder = httptest.NewRecorder()
	tr.mux.ServeHTTP(recorder, httptest.NewRequest("GET", "/transport", nil))
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Header().Get("Content-Type"), "json")
}
