package game

import (
	"time"

	"github.com/awdng/triebwerk/model"
)

const tickrate = 60

// Game represents the game state
type Game struct {
	tickStart      time.Time
	startTime      time.Time
	networkManager *NetworkManager
	playerManager  *PlayerManager
	state          model.GameState
}

// NewGame creates a game instance
func NewGame(networkManager *NetworkManager, playerManager *PlayerManager) *Game {
	return &Game{
		startTime:      time.Now(),
		networkManager: networkManager,
		playerManager:  playerManager,
		state: model.GameState{
			Players: make(map[uint8]*model.Player),
		},
	}
}

// GameTime returns the current game time since start in milliseconds
// TODO: review return type
func (g *Game) GameTime() uint32 {
	return uint32(time.Now().Sub(g.startTime) / time.Millisecond)
}

// RegisterPlayer registers a networked Player
func (g *Game) RegisterPlayer(conn model.Connection) {
	g.state.PlayerCount++
	player := g.playerManager.NewPlayer(g.state.PlayerCount, 10, 10, conn)
	g.networkManager.Register(player)
	g.state.Players[g.state.PlayerCount] = player
}

// Start the server update loop
func (g *Game) Start() error {
	// Execute game loop in a goroutine
	go func() {
		ticker := time.NewTicker(1000 / tickrate * time.Millisecond)
		for range ticker.C {
			g.tickStart = time.Now()

			// broadcast game state to clients
			g.networkManager.BroadcastGameState(g.state)
		}
	}()

	// Start networking
	return g.networkManager.Start()
}
