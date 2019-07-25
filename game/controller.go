package game

import (
	"log"
	"time"

	"github.com/awdng/triebwerk/model"
)

const tickrate = 30

var numMeasurements int64
var totalMeasurement int64
var avgTickTime float64

// Controller ...
type Controller struct {
	tickStart      time.Time
	networkManager *NetworkManager
	playerManager  *PlayerManager
	state          *model.GameState
}

// NewController creates a game instance
func NewController(networkManager *NetworkManager, playerManager *PlayerManager) *Controller {
	return &Controller{
		networkManager: networkManager,
		playerManager:  playerManager,
		state:          model.NewGameState(),
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
	g.Start()
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
	// goroutine constantly reads player input
	go g.processInputs()

	// Start networking
	return g.networkManager.Start()
}

// Start the gameserver loop
func (g *Controller) Start() {
	if g.state.ReadyToStart() {
		g.state.Start()
		// Execute game loop
		go g.gameLoop()
	}
}

func (g *Controller) processInputs() {
	// continously read all player inputs at 1000Hz
	ticker := time.NewTicker(1 * time.Millisecond)
	for range ticker.C {
		players := g.state.GetPlayers()
		for _, p := range players {
			select {
			case message := <-p.Client.NetworkIn:
				switch messageType := message.MessageType; messageType {
				case 1:
					p.Control = message.Body.(model.Controls)
				case 5:
					g.networkManager.SendTime(p, g.state, &message)
				default:
				}
			default:
			}
		}
	}
}

func (g *Controller) gameLoop() {
	gameEnded := make(chan bool)
	interval := time.Duration(int(1000/tickrate)) * time.Millisecond
	timestep := float32(interval/time.Millisecond) / 1000

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	log.Printf("GameManager: Game has started")
	for {
		select {
		case <-ticker.C:
			g.tickStart = time.Now()
			players := g.state.GetPlayers()

			// apply latest client inputs
			for _, p := range players {
				p.Update(players, g.state, timestep)
				p.HandleRespawn(g.state)
			}

			// broadcast game state to clients
			g.networkManager.BroadcastGameState(g.state)

			if g.state.HasEnded() {
				close(gameEnded)
			}

			// measure average tick time
			numMeasurements++
			totalMeasurement += time.Now().UTC().UnixNano() - g.tickStart.UTC().UnixNano()
			avgTickTime = float64(totalMeasurement/numMeasurements) / 1000 / 1000
		case _, ok := <-gameEnded:
			if !ok {
				g.state.End()
				log.Printf("GameManager: Game has ended")
				time.Sleep(10 * time.Second)
				g.Start()
				return
			}
		}
	}
}
