package game

import (
	"fmt"
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
}

// NewGame creates a game instance
func NewGame(networkManager *NetworkManager, playerManager *PlayerManager) *Game {
	return &Game{
		startTime:      time.Now(),
		networkManager: networkManager,
		playerManager:  playerManager,
	}
}

// GameTime returns the current game time since start in milliseconds
// TODO: review return type
func (g *Game) GameTime() uint32 {
	return uint32(time.Now().Sub(g.startTime) / time.Millisecond)
}

// RegisterPlayer registers a networked Player
func (g *Game) RegisterPlayer(conn model.Connection) {
	player := g.playerManager.NewPlayer(0, 10, 10, conn)
	g.networkManager.Register(player)
}

// Start the server update loop
func (g *Game) Start() error {
	// Execute game loop in a goroutine
	go func() {
		ticker := time.NewTicker(1000 / tickrate * time.Millisecond)
		for range ticker.C {
			g.tickStart = time.Now()
			fmt.Println("Game loop iteration")
		}
	}()

	// Start networking
	return g.networkManager.Start()
}
