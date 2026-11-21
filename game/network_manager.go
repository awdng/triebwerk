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

// Protocol is the wire format, see package protocol
type Protocol interface {
	Version() uint8
	// IsReliable says which lane a message of this type travels on
	IsReliable(messageType byte) bool
	Decode(data []byte) model.NetworkMessage
	Welcome(id int, tick uint32, roundLength time.Duration) []byte
	Reject() []byte
	Denied(reason byte) []byte
	RoundStart(tick uint32) []byte
	RoundEnd(tick uint32) []byte
	Scoreboard(board *model.Scoreboard) []byte
	Accounts(board *model.Scoreboard) []byte
	Kill(kill model.Kill) []byte
	Pong(clientTime uint32) []byte
	SnapshotBody(players []*model.Player) []byte
	Snapshot(tick uint32, ack uint32, waiting uint8, body []byte) []byte
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

// NetworkManager owns the connections: it reads what clients send, hands it
// to the game loop and writes what the game sends to them.
type NetworkManager struct {
	Ready bool

	// network context eg. websockets
	transport Transport

	// protocol that encodes/decodes data for network transfer
	protocol Protocol

	// Registered clients.
	clients map[*model.Client]bool

	// Register requests from the clients.
	register chan *model.Client

	// Unregister requests from clients.
	unregister chan *model.Client

	// Messages for clients, a batch is delivered in one go.
	outgoing chan []delivery
}

type delivery struct {
	client *model.Client
	data   []byte
}

// NewNetworkManager ...
func NewNetworkManager(transport Transport, protocol Protocol) *NetworkManager {
	return &NetworkManager{
		Ready:      false,
		transport:  transport,
		protocol:   protocol,
		register:   make(chan *model.Client),
		unregister: make(chan *model.Client),
		outgoing:   make(chan []delivery),
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
			n.remove(client)
		case batch := <-n.outgoing:
			for _, message := range batch {
				if _, ok := n.clients[message.client]; ok {
					n.deliver(message.client, message.data)
				}
			}
		}
	}
}

// deliver queues data for a client without blocking.
// The client is disconnected if its network output channel buffer reaches maximum size.
// Must only be called from run, which owns the NetworkOut channels.
func (n *NetworkManager) deliver(client *model.Client, data []byte) {
	select {
	case client.NetworkOut <- data:
	default:
		log.Printf("NetworkManager: Closing connection of Client %s: Could not write to NetworkOut channel, buffer size %d", client.Connection.Identifier(), len(client.NetworkOut))
		n.remove(client)
	}
}

// remove a client, must only be called from run
func (n *NetworkManager) remove(client *model.Client) {
	if _, ok := n.clients[client]; !ok {
		return
	}
	client.Disconnect()
	delete(n.clients, client)
	log.Printf("NetworkManager: Client %s disconnected, %d connected clients ", client.Connection.Identifier(), len(n.clients))
	n.transport.Unregister(client.Connection)
}

// Version of the wire format a client has to speak
func (n *NetworkManager) Version() uint8 {
	return n.protocol.Version()
}

// Register starts reading from and writing to a connection that just arrived
func (n *NetworkManager) Register(client *model.Client) {
	n.register <- client
}

// Disconnect a client. What was sent to it before still goes out.
func (n *NetworkManager) Disconnect(client *model.Client) {
	n.unregister <- client
}

// Send data to a client
func (n *NetworkManager) Send(client *model.Client, message []byte) {
	n.outgoing <- []delivery{{client: client, data: message}}
}

// sendAll sends the same message to every player
func (n *NetworkManager) sendAll(players []*model.Player, message []byte) {
	if len(players) == 0 {
		return
	}
	batch := make([]delivery, len(players))
	for i, p := range players {
		batch[i] = delivery{client: p.Client, data: message}
	}
	n.outgoing <- batch
}

// Welcome tells a client that it is a player now
func (n *NetworkManager) Welcome(player *model.Player, state *model.GameState) {
	n.Send(player.Client, n.protocol.Welcome(player.ID, state.TickNumber(), state.RoundLength()))
}

// Reject turns a client away that speaks another version of the protocol
func (n *NetworkManager) Reject(client *model.Client) {
	n.Send(client, n.protocol.Reject())
	n.Disconnect(client)
}

// Deny turns a player away that the master server did not vouch for
func (n *NetworkManager) Deny(client *model.Client, reason byte) {
	n.Send(client, n.protocol.Denied(reason))
	n.Disconnect(client)
}

// SendSnapshots sends every player the state of all tanks after the current
// tick, each with the number of its own last input that went into it
func (n *NetworkManager) SendSnapshots(state *model.GameState) {
	players := state.GetPlayers()
	if len(players) == 0 {
		return
	}
	body := n.protocol.SnapshotBody(players)
	tick := state.TickNumber()
	batch := make([]delivery, len(players))
	for i, p := range players {
		batch[i] = delivery{client: p.Client, data: n.protocol.Snapshot(tick, p.Inputs.Applied(), p.InputsWaiting(), body)}
	}
	n.outgoing <- batch
}

// BroadcastRoundStart ...
func (n *NetworkManager) BroadcastRoundStart(state *model.GameState) {
	n.sendAll(state.GetPlayers(), n.protocol.RoundStart(state.RoundStartTick()))
}

// SendRoundStart tells a player that joined a running round when it started
func (n *NetworkManager) SendRoundStart(client *model.Client, state *model.GameState) {
	n.Send(client, n.protocol.RoundStart(state.RoundStartTick()))
}

// BroadcastRoundEnd ...
func (n *NetworkManager) BroadcastRoundEnd(state *model.GameState) {
	n.sendAll(state.GetPlayers(), n.protocol.RoundEnd(state.TickNumber()))
}

// BroadcastScoreboard tells every player the names, kills and deaths of all
// players, and right behind it who of them plays with an account
func (n *NetworkManager) BroadcastScoreboard(state *model.GameState) {
	board := state.Scoreboard()
	players := state.GetPlayers()
	n.sendAll(players, n.protocol.Scoreboard(board))
	n.sendAll(players, n.protocol.Accounts(board))
}

// BroadcastKill tells every player who destroyed whom
func (n *NetworkManager) BroadcastKill(state *model.GameState, killer int, victim int) {
	n.sendAll(state.GetPlayers(), n.protocol.Kill(model.Kill{Killer: killer, Victim: victim}))
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

			var err error
			if len(message) == 0 || n.protocol.IsReliable(message[0]) {
				err = client.Connection.Write(message)
			} else {
				err = client.Connection.WriteUnreliable(message)
			}
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
		decoded := n.protocol.Decode(message)
		// answered here and not by the game loop: the round trip a client
		// measures does not include the wait for the next tick
		if ping, ok := decoded.Body.(model.Ping); ok {
			n.Send(client, n.protocol.Pong(ping.ClientTime))
			continue
		}
		// never block here, a blocked reader stops answering pings and the client times out
		select {
		case client.NetworkIn <- decoded:
		default:
			log.Printf("Reader: Dropping message of Client %s: NetworkIn channel is full", client.Connection.Identifier())
		}
	}
}
