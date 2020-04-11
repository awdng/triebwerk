package game

import (
	"log"
	"time"

	"github.com/awdng/triebwerk"
	"github.com/awdng/triebwerk/model"
)

const tickrate = 30

var numMeasurements int64
var totalMeasurement int64
var avgTickTime float64

type serverState struct {
	Connect   string            `firestore:"public_ip"`
	Scores    map[string]int    `firestore:"scores"`
	Names     map[string]string `firestore:"names"`
	GameTime  int               `firestore:"gametime"`
	UpdatedAt int64             `firestore:"updated_at"`
}

// Controller ...
type Controller struct {
	tickStart      time.Time
	networkManager *NetworkManager
	playerManager  *PlayerManager
	state          *model.GameState
	firebase       *triebwerk.Firebase
	masterServer   MasterServerClient
}

// MasterServerClient ...
type MasterServerClient interface {
	Init(address string)
	GetServerState()
	SendHeartbeat(*model.GameState)
	EndGame(*model.GameState)
	AuthorizePlayer(string, *model.Player) error
}

// NewController creates a game instance
func NewController(region string, networkManager *NetworkManager, playerManager *PlayerManager, firebase *triebwerk.Firebase, masterServer MasterServerClient) *Controller {
	return &Controller{
		networkManager: networkManager,
		playerManager:  playerManager,
		state:          model.NewGameState(region),
		firebase:       firebase,
		masterServer:   masterServer,
	}
}

// RegisterPlayer registers a networked Player
func (g *Controller) RegisterPlayer(conn model.Connection) {
	players := g.state.GetPlayers()
	pID := g.state.GetNewPlayerID()
	spawn := g.state.Map.GetRandomSpawn(players)
	player := model.NewPlayer(pID, spawn.X, spawn.Y, conn)
	g.networkManager.Register(player, g.state)
	g.state.AddPlayer(player)
	g.CheckStartConditions()
	if g.state.InProgress() { // game already started, new Player has to know about it
		g.networkManager.SendGameStartToClient(player.Client, g.state)
	}
	log.Printf("GameManager: Player %d connected, %d connected Players", player.ID, g.state.GetPlayerCount())
}

// UnregisterPlayer of a networked game
func (g *Controller) UnregisterPlayer(conn model.Connection) {
	players := g.state.GetPlayers()
	for _, p := range players {
		if p.Client.Connection == conn {
			g.state.RemovePlayer(p)
			log.Printf("GameManager: Player %d disconnected, %d connected Players", p.ID, g.state.GetPlayerCount())
			break
		}
	}
}

// Init the gameserver
func (g *Controller) Init() error {
	// init HeartBeat
	go g.HeartBeat()

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
		g.masterServer.SendHeartbeat(g.state)
	}
}

// CheckStartConditions the gameserver loop
func (g *Controller) CheckStartConditions() {
	if g.state.ReadyToStart() {
		g.state.Start()
		// Execute game loop
		go g.gameLoop()
	}
}

func (g *Controller) processInputs(p *model.Player, players []*model.Player, timestep float32) {
	// read control input
	for len(p.Client.NetworkIn) != 0 {
		message := <-p.Client.NetworkIn
		switch messageType := message.MessageType; messageType {
		case 0:
			token := message.Body.(string)
			err := g.masterServer.AuthorizePlayer(token, p)
			// err := g.playerManager.Authorize(p, token)
			if err != nil {
				log.Printf("GameManager: Player %d (%s) could not be authorized, forcing disconnect: %s", p.ID, p.GlobalID, err)
				g.networkManager.ForceDisconnect(p)
				continue
			}
			log.Printf("GameManager: Player %d authorized successfully as GlobalID %s %s", p.ID, p.GlobalID, p.Nickname)
		case 1:
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

func (g *Controller) gameLoop() {
	interval := time.Duration(int(1000/tickrate)) * time.Millisecond
	timestep := float32(interval/time.Millisecond) / 1000

	ticker := time.NewTicker(interval)
	g.networkManager.BroadcastGameStart(g.state)
	log.Printf("GameManager: Game has started")
	for range ticker.C {
		g.tickStart = time.Now()
		players := g.state.GetPlayers()

		// apply latest client inputs
		for _, p := range players {
			g.processInputs(p, players, timestep)
			p.HandleRespawn(g.state)
		}

		// broadcast game state to clients
		g.networkManager.BroadcastGameState(g.state)

		if g.state.HasEnded() {
			break
		}

		// measure average tick time
		numMeasurements++
		totalMeasurement += time.Now().UTC().UnixNano() - g.tickStart.UTC().UnixNano()
		avgTickTime = float64(totalMeasurement/numMeasurements) / 1000 / 1000
	}
	ticker.Stop()

	g.state.End()
	log.Printf("GameManager: Game has ended")
	g.networkManager.BroadcastGameEnd(g.state)
	g.masterServer.EndGame(g.state)
	time.Sleep(10 * time.Second)
	g.CheckStartConditions()
}
