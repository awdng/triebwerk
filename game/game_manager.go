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
		state: &model.GameState{
			StartTime: time.Now(),
			Players:   make(map[uint8]*model.Player),
		},
	}
}

// RegisterPlayer registers a networked Player
func (g *Game) RegisterPlayer(conn model.Connection) {
	g.state.PlayerCount++
	player := g.playerManager.NewPlayer(g.state.PlayerCount, 10*float32(g.state.PlayerCount), 0, conn)
	g.networkManager.Register(player, g.state)
	g.state.Players[g.state.PlayerCount] = player
	log.Printf("GameManager: Player %d connected, %d connected Players", player.ID, g.state.PlayerCount)
}

// UnregisterPlayer of a networked game
func (g *Game) UnregisterPlayer(conn model.Connection) {
	for _, p := range g.state.Players {
		if p.Client.Connection == conn {
			g.state.PlayerCount--
			delete(g.state.Players, p.ID)
			log.Printf("GameManager: Player %d disconnected, %d connected Players", p.ID, g.state.PlayerCount)
		}
	}
}

// Start the gameserver loop
func (g *Game) Start() error {
	// goroutine constantly reads player input
	go func() {
		for {
			for _, p := range g.state.Players {
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
	}()

	// Execute game loop
	go func() {
		ticker := time.NewTicker(33 * time.Millisecond)
		for range ticker.C {
			g.tickStart = time.Now()

			if len(g.state.Players) == 0 {
				continue
			}

			// apply latest client inputs
			for _, p := range g.state.Players {
				p.ApplyMovement(p.Control)
			}

			// broadcast game state to clients
			g.networkManager.BroadcastGameState(g.state)
		}
	}()

	// Start networking
	return g.networkManager.Start()
}
