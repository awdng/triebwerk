package game

import (
	"fmt"
	"log"
	"time"

	"github.com/awdng/triebwerk/model"
)

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer.
	maxMessageSize = 512
)

// Protocol that encodes/decodes data for network transfer
type Protocol interface {
	Encode(p *model.Player, currentGameTime uint32, messageType uint8) []byte
	Decode(data []byte) model.NetworkMessage
}

// Transport represents the network context
type Transport interface {
	Init()
	Run() error
	RegisterNewConnHandler(register func(conn model.Connection))
}

// NetworkManager maintains the set of active clients and broadcasts messages to the
// clients.
type NetworkManager struct {
	// network context eg. websockets
	transport Transport

	// protocol that encodes/decodes data for network transfer
	protocol Protocol

	// Registered clients.
	clients map[*model.Client]bool

	// Inbound messages from the clients.
	broadcast chan []byte

	// Register requests from the clients.
	register chan *model.Client

	// Unregister requests from clients.
	unregister chan *model.Client
}

// NewNetworkManager ...
func NewNetworkManager(transport Transport, protocol Protocol) *NetworkManager {
	return &NetworkManager{
		transport:  transport,
		protocol:   protocol,
		broadcast:  make(chan []byte),
		register:   make(chan *model.Client),
		unregister: make(chan *model.Client),
		clients:    make(map[*model.Client]bool),
	}
}

// Start handling network connections
func (n *NetworkManager) Start() error {
	n.transport.Init()
	go n.run()
	return n.transport.Run()
}

func (n *NetworkManager) run() {
	log.Printf("Listening for incoming Network traffic ...")
	for {
		select {
		case client := <-n.register:
			n.clients[client] = true
			go n.writer(client)
			go n.reader(client)
			log.Printf("Client %s connected, %d connected clients ", client.Connection.Identifier(), len(n.clients))
		case client := <-n.unregister:
			if _, ok := n.clients[client]; ok {
				client.Disconnect()
				delete(n.clients, client)
				log.Printf("Client %s disconnected, %d connected clients ", client.Connection.Identifier(), len(n.clients))
			}
		case message := <-n.broadcast:
			for client := range n.clients {
				select {
				case client.NetworkOut <- message:
				default:
					client.Disconnect()
					delete(n.clients, client)
				}
			}
		}
	}
}

// Register a new CLient with the NetworkService
func (n *NetworkManager) Register(client *model.Client) {
	n.register <- client
}

// Send data to a client
func (n *NetworkManager) Send(client *model.Client, message []byte) error {
	if _, ok := n.clients[client]; !ok {
		return fmt.Errorf("Client not found %d", client.Connection)
	}

	// TODO: implement encoding of player state
	// Message needs to receive a struct that is encoded before sending to networkOut
	// struct needs to include MessageType
	//n.Protocol.Encode(player, ...)

	client.NetworkOut <- message
	return nil
}

// BroadcastGameState ...
func (n *NetworkManager) BroadcastGameState(state model.GameState) {
	buf := make([]byte, 0)
	for _, p := range state.Players {
		buf = append(buf, n.protocol.Encode(p, state.GameTime(), 1)...)
	}
	if len(buf) > 0 {
		fmt.Println(buf)
		n.broadcast <- buf
	}
}

// Writer constantly reads messages from the players NetworkOut and sends it to the websocket connection.
//
// A goroutine running Writer is started for each connection. The
// application ensures that there is at most one writer to a connection by
// executing all writes from this goroutine.
func (n *NetworkManager) writer(client *model.Client) {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		client.Connection.Close(writeWait, false)
	}()
	for {
		select {
		case message, ok := <-client.NetworkOut:
			client.Connection.PrepareWrite(writeWait)
			if !ok {
				// The NetworkManager closed the channel.
				client.Connection.Close(writeWait, true)
				return
			}

			err := client.Connection.Write(message)
			if err != nil {
				return
			}
		case <-ticker.C:
			client.Connection.Ping(writeWait)
		}
	}
}

// Reader constantly reads messages from the websocket connection and passes them to the NetworkManager.
//
// The application runs reader in a per-connection goroutine. The application
// ensures that there is at most one reader on a connection by executing all
// reads from this goroutine.
func (n *NetworkManager) reader(client *model.Client) {
	defer func() {
		n.unregister <- client
		client.Connection.Close(writeWait, false)
	}()
	client.Connection.PrepareRead(maxMessageSize, pongWait)
	for {
		message, err := client.Connection.Read()
		if err != nil {
			// connection will be closed
			break
		}

		client.NetworkIn <- n.protocol.Decode(message)
	}
}
