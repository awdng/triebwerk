package model

import (
	"sync"
	"time"
)

// GameState ...
type GameState struct {
	StartTime   time.Time
	PlayerCount uint8
	Players     map[uint8]*Player
	Map         *Map
	mutex       *sync.Mutex
}

// NewGameState ...
func NewGameState() *GameState {
	return &GameState{
		StartTime: time.Now(),
		Players:   make(map[uint8]*Player),
		Map:       NewMap(),
		mutex:     &sync.Mutex{},
	}
}

// GameTime returns the current game time since start in milliseconds
func (g *GameState) GameTime() uint32 {
	return uint32(time.Now().Sub(g.StartTime) / time.Millisecond)
}

// GetPlayers returns the PlayerList
func (g *GameState) GetPlayers() []*Player {
	players := make([]*Player, 0)
	g.mutex.Lock()
	for _, p := range g.Players {
		players = append(players, p)
	}
	g.mutex.Unlock()

	return players
}
