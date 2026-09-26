package game

import (
	"log"
	"sync"
	"time"

	"github.com/awdng/triebwerk/model"
)

const tickrate = 30

// time between the end of a game and the start of the next one
const intermission = 10 * time.Second

var numMeasurements int64
var totalMeasurement int64
var avgTickTime float64

// events that are created outside of the game loop and processed inside of it
type joinEvent struct {
	conn model.Connection
}

type leaveEvent struct {
	conn model.Connection
}

type authEvent struct {
	player   *model.Player
	globalID string
	nickname string
	err      error
}

// Controller ...
// The game state is owned by the game loop goroutine, other goroutines
// communicate with it through events.
type Controller struct {
	tickStart      time.Time
	networkManager *NetworkManager
	state          *model.GameState
	masterServer   MasterServerClient
	maxPlayers     int
	nextStart      time.Time

	eventMutex sync.Mutex
	events     []interface{}

	snapshotMutex sync.Mutex
	snapshot      model.StateSnapshot
}

// MasterServerClient ...
type MasterServerClient interface {
	Init(address string)
	GetServerState()
	SendHeartbeat(model.StateSnapshot)
	EndGame(model.StateSnapshot)
	AuthorizePlayer(token string) (globalID string, nickname string, err error)
}

// NewController creates a game instance
func NewController(region string, maxPlayers int, gameLength time.Duration, networkManager *NetworkManager, masterServer MasterServerClient) *Controller {
	state := model.NewGameState(region, gameLength)
	return &Controller{
		networkManager: networkManager,
		state:          state,
		masterServer:   masterServer,
		maxPlayers:     maxPlayers,
		snapshot:       state.Snapshot(),
	}
}

// RegisterPlayer registers a networked Player
func (g *Controller) RegisterPlayer(conn model.Connection) {
	g.pushEvent(joinEvent{conn: conn})
}

// UnregisterPlayer of a networked game
func (g *Controller) UnregisterPlayer(conn model.Connection) {
	g.pushEvent(leaveEvent{conn: conn})
}

// pushEvent never blocks, so it is safe to call from the network goroutines
func (g *Controller) pushEvent(event interface{}) {
	g.eventMutex.Lock()
	defer g.eventMutex.Unlock()
	g.events = append(g.events, event)
}

func (g *Controller) popEvents() []interface{} {
	g.eventMutex.Lock()
	defer g.eventMutex.Unlock()
	events := g.events
	g.events = nil
	return events
}

// Init the gameserver
func (g *Controller) Init() error {
	// init HeartBeat
	go g.HeartBeat()

	go g.gameLoop()

	// Start networking
	return g.networkManager.Start()
}

// HeartBeat ...
func (g *Controller) HeartBeat() {
	// Wait for Network to become ready
	time.Sleep(time.Second)

	// log.Printf("GameManager: Server Registered with global ID %s", server.ID)

	ticker := time.NewTicker(time.Second * 5)
	g.masterServer.Init(g.networkManager.GetAddress())
	for range ticker.C {
		g.masterServer.SendHeartbeat(g.latestSnapshot())
	}
}

func (g *Controller) latestSnapshot() model.StateSnapshot {
	g.snapshotMutex.Lock()
	defer g.snapshotMutex.Unlock()
	return g.snapshot
}

func (g *Controller) handleEvents() {
	for _, event := range g.popEvents() {
		switch e := event.(type) {
		case joinEvent:
			g.addPlayer(e.conn)
		case leaveEvent:
			g.removePlayer(e.conn)
		case authEvent:
			g.finishAuthorization(e)
		}
	}
}

func (g *Controller) addPlayer(conn model.Connection) {
	if g.state.GetPlayerCount() >= g.maxPlayers {
		log.Printf("GameManager: Rejecting connection %s, server is full (%d Players)", conn.Identifier(), g.maxPlayers)
		conn.Close(writeWait, true)
		conn.Close(writeWait, false)
		return
	}

	players := g.state.GetPlayers()
	pID := g.state.GetNewPlayerID()
	spawn := g.state.Map.GetRandomSpawn(players)
	player := model.NewPlayer(pID, spawn.X, spawn.Y, conn)
	g.networkManager.Register(player, g.state)
	g.state.AddPlayer(player)
	if g.state.InProgress() { // game already started, new Player has to know about it
		g.networkManager.SendGameStartToClient(player.Client, g.state)
	}
	log.Printf("GameManager: Player %d connected, %d connected Players", player.ID, g.state.GetPlayerCount())
}

func (g *Controller) removePlayer(conn model.Connection) {
	for _, p := range g.state.GetPlayers() {
		if p.Client.Connection == conn {
			g.state.RemovePlayer(p)
			log.Printf("GameManager: Player %d disconnected, %d connected Players", p.ID, g.state.GetPlayerCount())
			return
		}
	}
}

// authorize asks the master server in the background so the game loop does not wait for it
func (g *Controller) authorize(p *model.Player, token string) {
	go func() {
		globalID, nickname, err := g.masterServer.AuthorizePlayer(token)
		g.pushEvent(authEvent{player: p, globalID: globalID, nickname: nickname, err: err})
	}()
}

func (g *Controller) finishAuthorization(e authEvent) {
	p := e.player
	if !g.isConnected(p) {
		return
	}
	if e.err != nil {
		log.Printf("GameManager: Player %d (%s) could not be authorized, forcing disconnect: %s", p.ID, p.GlobalID, e.err)
		g.networkManager.ForceDisconnect(p)
		return
	}
	p.GlobalID = e.globalID
	p.Nickname = e.nickname
	log.Printf("GameManager: Player %d authorized successfully as GlobalID %s %s", p.ID, p.GlobalID, p.Nickname)
}

func (g *Controller) isConnected(player *model.Player) bool {
	for _, p := range g.state.GetPlayers() {
		if p == player {
			return true
		}
	}
	return false
}

func (g *Controller) processInputs(p *model.Player, players []*model.Player, timestep float32) {
	// the fired flag is reported to the clients once per tick
	p.Fired = false
	// read control input
	for len(p.Client.NetworkIn) != 0 {
		message := <-p.Client.NetworkIn
		switch messageType := message.MessageType; messageType {
		case 0:
			g.authorize(p, message.Body.(string))
		case 1:
			// inputs between games are discarded
			if !g.state.InProgress() {
				continue
			}
			// make sure all input gets processed
			p.Control = message.Body.(model.Controls)
			p.Update(players, g.state, timestep)
		case 5:
			g.networkManager.SendTime(p, g.state, &message)
		}
	}
	if len(p.Client.NetworkIn) > 1 {
		log.Printf("WARNING: GameManager: Applied more than 1 input for Player %d with GlobalID %s", p.ID, p.GlobalID)
	}
}

func (g *Controller) startGame() {
	g.state.Start()
	g.networkManager.BroadcastGameStart(g.state)
	log.Printf("GameManager: Game has started")
}

func (g *Controller) endGame() {
	g.state.End()
	log.Printf("GameManager: Game has ended")
	g.networkManager.BroadcastGameEnd(g.state)
	go g.masterServer.EndGame(g.state.Snapshot())
	g.nextStart = time.Now().Add(intermission)
}

// gameLoop is the only goroutine that modifies the game state
func (g *Controller) gameLoop() {
	interval := time.Duration(int(1000/tickrate)) * time.Millisecond
	timestep := float32(interval/time.Millisecond) / 1000

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		g.tickStart = time.Now()
		g.handleEvents()

		if !g.state.InProgress() && g.state.ReadyToStart() && !g.tickStart.Before(g.nextStart) {
			g.startGame()
		}

		players := g.state.GetPlayers()

		// apply latest client inputs
		for _, p := range players {
			g.processInputs(p, players, timestep)
			if g.state.InProgress() {
				p.HandleRespawn(g.state)
			}
		}

		if g.state.InProgress() {
			// broadcast game state to clients
			g.networkManager.BroadcastGameState(g.state)

			if g.state.HasEnded() {
				g.endGame()
			}
		}

		g.snapshotMutex.Lock()
		g.snapshot = g.state.Snapshot()
		g.snapshotMutex.Unlock()

		// measure average tick time
		numMeasurements++
		totalMeasurement += time.Now().UTC().UnixNano() - g.tickStart.UTC().UnixNano()
		avgTickTime = float64(totalMeasurement/numMeasurements) / 1000 / 1000
	}
}
