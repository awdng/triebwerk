package game

import (
	"log"
	"time"

	"github.com/awdng/triebwerk/model"
)

const (
	// Time allowed to write a message to the peer.
	writeWait = 1 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 5 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer.
	maxMessageSize = 1024
)

// MessageType ...
type MessageType uint8

const (
	spawn MessageType = iota
	position
	register
	projectile
	hit
	serverTime
	gameStart
	gameEnd
)

// Protocol that encodes/decodes data for network transfer
type Protocol interface {
	Encode(id int, currentGameTime uint32, message *model.NetworkMessage) []byte
	Decode(data []byte) model.NetworkMessage
}

// Transport represents the network context
type Transport interface {
	Init()
	GetAddress() string
	Run() error
	RegisterNewConnHandler(register func(conn model.Connection))
	UnregisterConnHandler(unregister func(conn model.Connection))
	Unregister(conn model.Connection)
}

// NetworkManager maintains the set of active clients and broadcasts messages to the
// clients.
type NetworkManager struct {
	Ready bool

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
		Ready:      false,
		transport:  transport,
		protocol:   protocol,
		broadcast:  make(chan []byte),
		register:   make(chan *model.Client),
		unregister: make(chan *model.Client),
		clients:    make(map[*model.Client]bool),
	}
}

// GetAddress ...
func (n *NetworkManager) GetAddress() string {
	return n.transport.GetAddress()
}

// Start handling network connections
func (n *NetworkManager) Start() error {
	n.transport.Init()
	go n.run()
	return n.transport.Run()
}

func (n *NetworkManager) run() {
	log.Printf("NetworkManager: Listening for incoming Network traffic ...")
	for {
		select {
		case client := <-n.register:
			n.clients[client] = true
			go n.writer(client)
			go n.reader(client)
			log.Printf("NetworkManager: Client %s connected, %d connected clients ", client.Connection.Identifier(), len(n.clients))
		case client := <-n.unregister:
			if _, ok := n.clients[client]; ok {
				client.Disconnect()
				delete(n.clients, client)
				log.Printf("NetworkManager: Client %s disconnected, %d connected clients ", client.Connection.Identifier(), len(n.clients))
				n.transport.Unregister(client.Connection)
			}
		case message := <-n.broadcast:
			for client := range n.clients {
				// select is used to avoid blocking when a network output writer of a client is not ready
				// client is disconnectet if network output channel buffer reaches maximum size
				select {
				case client.NetworkOut <- message:
				default:
					log.Printf("NetworkManager: Closing connection of Client %s: Could not write to NetworkOut channel, buffer size %d", client.Connection.Identifier(), len(client.NetworkOut))
					n.unregister <- client
				}
			}
		}
	}
}

// Register a new Client with the NetworkService
func (n *NetworkManager) Register(player *model.Player, state *model.GameState) {
	n.register <- player.Client

	// send registration confirmation to client
	buf := make([]byte, 0)
	buf = append(buf, n.protocol.Encode(player.ID, state.GameTime(), &model.NetworkMessage{
		MessageType: uint8(register),
	})...)
	n.Send(player.Client, buf)
}

// ForceDisconnect of Player
func (n *NetworkManager) ForceDisconnect(player *model.Player) {
	client := player.Client
	client.Connection.Close(writeWait, false)
	n.unregister <- client
}

// SendTime back to player
func (n *NetworkManager) SendTime(player *model.Player, state *model.GameState, message *model.NetworkMessage) {
	buf := make([]byte, 0)
	buf = append(buf, n.protocol.Encode(player.ID, state.GameTime(), message)...)
	n.Send(player.Client, buf)
}

// Send data to a client
func (n *NetworkManager) Send(client *model.Client, message []byte) error {
	client.NetworkOut <- message
	return nil
}

// BroadcastGameState ...
func (n *NetworkManager) BroadcastGameState(state *model.GameState) {
	buf := make([]byte, 0)
	players := state.GetPlayers()
	for _, p := range players {
		buf = append(buf, n.protocol.Encode(p.ID, state.GameTime(), &model.NetworkMessage{
			MessageType: uint8(position),
			Body:        p,
		})...)
	}
	if len(buf) > 0 {
		n.broadcast <- buf
	}
}

// BroadcastGameStart ...
func (n *NetworkManager) BroadcastGameStart(state *model.GameState) {
	buf := n.protocol.Encode(0, state.GameTime(), &model.NetworkMessage{
		MessageType: uint8(gameStart),
	})

	if len(buf) > 0 {
		n.broadcast <- buf
	}
}

// SendGameStartToClient ...
func (n *NetworkManager) SendGameStartToClient(client *model.Client, state *model.GameState) {
	buf := n.protocol.Encode(0, state.GameTime(), &model.NetworkMessage{
		MessageType: uint8(gameStart),
	})

	if len(buf) > 0 {
		n.Send(client, buf)
	}
}

// BroadcastGameEnd ...
func (n *NetworkManager) BroadcastGameEnd(state *model.GameState) {
	buf := n.protocol.Encode(0, state.GameTime(), &model.NetworkMessage{
		MessageType: uint8(gameEnd),
	})

	if len(buf) > 0 {
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
				log.Printf("Writer: Could not read NetworkOut Channel of Client %s", client.Connection.Identifier())
				client.Connection.Close(writeWait, true)
				return
			}

			err := client.Connection.Write(message)
			if err != nil {
				log.Printf("Writer: Closing connection for Client %s: %s", client.Connection.Identifier(), err)
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
		client.Connection.Close(writeWait, false)
		n.unregister <- client
	}()
	client.Connection.PrepareRead(maxMessageSize, pongWait)
	for {
		message, err := client.Connection.Read()
		if err != nil {
			// connection will be closed
			log.Printf("Reader: Closing connection of Client %s: %s", client.Connection.Identifier(), err)
			break
		}
		client.NetworkIn <- n.protocol.Decode(message)
	}
}
