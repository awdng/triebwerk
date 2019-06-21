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
	Encode(p *model.Player, currentGameTime uint32, messageType int8) []byte
	Decode(data []byte, p *model.Player)
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
	clients map[*model.Player]bool

	// Inbound messages from the clients.
	broadcast chan []byte

	// Register requests from the clients.
	register chan *model.Player

	// Unregister requests from clients.
	unregister chan *model.Player
}

// NewNetworkManager ...
func NewNetworkManager(transport Transport, protocol Protocol) *NetworkManager {
	return &NetworkManager{
		transport:  transport,
		protocol:   protocol,
		broadcast:  make(chan []byte),
		register:   make(chan *model.Player),
		unregister: make(chan *model.Player),
		clients:    make(map[*model.Player]bool),
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
		case client := <-n.unregister:
			if _, ok := n.clients[client]; ok {
				client.Disconnect()
				delete(n.clients, client)
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

// Register a new Player with the NetworkService
func (n *NetworkManager) Register(player *model.Player) {
	n.register <- player
}

// Send data to a client
func (n *NetworkManager) Send(player *model.Player, message []byte) error {
	if _, ok := n.clients[player]; !ok {
		return fmt.Errorf("Client not found %d", player.ID)
	}

	// TODO: implement encoding of player state
	// Message needs to receive a struct that is encoded before sending to networkOut
	// struct needs to include MessageType
	//n.Protocol.Encode(player, ...)

	player.NetworkOut <- message
	return nil
}

// Writer constantly reads messages from the players NetworkOut and sends it to the websocket connection.
//
// A goroutine running Writer is started for each connection. The
// application ensures that there is at most one writer to a connection by
// executing all writes from this goroutine.
func (n *NetworkManager) writer(player *model.Player) {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		player.Connection.Close(writeWait, false)
	}()
	for {
		select {
		case message, ok := <-player.NetworkOut:
			player.Connection.PrepareWrite(writeWait)
			if !ok {
				// The NetworkManager closed the channel.
				player.Connection.Close(writeWait, true)
				return
			}

			err := player.Connection.Write(message)
			if err != nil {
				return
			}
		case <-ticker.C:
			player.Connection.Ping(writeWait)
		}
	}
}

// Reader constantly reads messages from the websocket connection and passes them to the NetworkManager.
//
// The application runs reader in a per-connection goroutine. The application
// ensures that there is at most one reader on a connection by executing all
// reads from this goroutine.
func (n *NetworkManager) reader(player *model.Player) {
	defer func() {
		n.unregister <- player
		player.Connection.Close(writeWait, false)
	}()
	player.Connection.PrepareRead(maxMessageSize, pongWait)
	for {
		message, err := player.Connection.Read()
		if err != nil {
			// connection will be closed
			break
		}
		// update player state
		n.protocol.Decode(message, player)
		fmt.Println(player.Control)
	}
}
