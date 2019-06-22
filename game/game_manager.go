package game

import (
	"time"

	"github.com/awdng/triebwerk/model"
)

const tickrate = 5

// Game represents the game state
type Game struct {
	tickStart      time.Time
	networkManager *NetworkManager
	playerManager  *PlayerManager
	state          model.GameState
}

// NewGame creates a game instance
func NewGame(networkManager *NetworkManager, playerManager *PlayerManager) *Game {
	return &Game{
		networkManager: networkManager,
		playerManager:  playerManager,
		state: model.GameState{
			StartTime: time.Now(),
			Players:   make(map[uint8]*model.Player),
		},
	}
}

// RegisterPlayer registers a networked Player
func (g *Game) RegisterPlayer(conn model.Connection) {
	g.state.PlayerCount++
	player := g.playerManager.NewPlayer(g.state.PlayerCount, 10*float32(g.state.PlayerCount), 10, conn)
	g.networkManager.Register(player.Client)
	g.state.Players[g.state.PlayerCount] = player
}

// Start the server update loop
func (g *Game) Start() error {
	// Execute game loop
	go func() {
		ticker := time.NewTicker(1000 / tickrate * time.Millisecond)
		for range ticker.C {
			g.tickStart = time.Now()

			if len(g.state.Players) == 0 {
				continue
			}

			// read client inputs
			for _, p := range g.state.Players {
				message := <-p.Client.NetworkIn
				p.ApplyMovement(message.Body.(model.Controls))
			}

			// broadcast game state to clients
			g.networkManager.BroadcastGameState(g.state)
		}
	}()

	// Start networking
	return g.networkManager.Start()
}
