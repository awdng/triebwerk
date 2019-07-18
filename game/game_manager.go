package game

import (
	"log"
	"time"

	"github.com/awdng/triebwerk/model"
)

const tickrate = 30

// Game represents the game state
type Game struct {
	tickStart      time.Time
	networkManager *NetworkManager
	playerManager  *PlayerManager
	state          *model.GameState
}

// NewGame creates a game instance
func NewGame(networkManager *NetworkManager, playerManager *PlayerManager) *Game {
	return &Game{
		networkManager: networkManager,
		playerManager:  playerManager,
		state:          model.NewGameState(),
	}
}

// RegisterPlayer registers a networked Player
func (g *Game) RegisterPlayer(conn model.Connection) {
	pID := g.state.GetNewPlayerID()
	spawn := g.state.Map.GetRandomSpawn()
	player := model.NewPlayer(pID, spawn.X, spawn.Y, conn)
	g.networkManager.Register(player, g.state)
	g.state.AddPlayer(player)
	log.Printf("GameManager: Player %d connected, %d connected Players", player.ID, g.state.GetPlayerCount())
}

// UnregisterPlayer of a networked game
func (g *Game) UnregisterPlayer(conn model.Connection) {
	players := g.state.GetPlayers()
	for _, p := range players {
		if p.Client.Connection == conn {
			g.state.RemovePlayer(p)
			log.Printf("GameManager: Player %d disconnected, %d connected Players", p.ID, g.state.GetPlayerCount())
			break
		}
	}
}

// Start the gameserver loop
func (g *Game) Start() error {
	// goroutine constantly reads player input
	go g.processInputs()

	// Execute game loop
	go g.gameLoop()

	// Start networking
	return g.networkManager.Start()
}

func (g *Game) processInputs() {
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

func (g *Game) gameLoop() {
	interval := time.Duration(int(1000/tickrate)) * time.Millisecond
	ticker := time.NewTicker(interval)
	timestep := float32(interval/time.Millisecond) / 1000
	for range ticker.C {
		g.tickStart = time.Now()
		players := g.state.GetPlayers()

		if len(players) == 0 {
			continue
		}

		// apply latest client inputs
		for _, p := range players {
			p.Update(players, g.state, timestep)
			p.HandleRespawn(g.state)
		}

		// broadcast game state to clients
		g.networkManager.BroadcastGameState(g.state)
	}
}
