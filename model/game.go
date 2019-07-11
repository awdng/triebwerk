package model

import (
	"sync"
	"sync/atomic"
	"time"
)

// GameState ...
type GameState struct {
	StartTime   time.Time
	PlayerCount int64
	Players     map[int]*Player
	Map         *Map
	mutex       *sync.Mutex
}

// NewGameState ...
func NewGameState() *GameState {
	return &GameState{
		StartTime: time.Now(),
		Players:   make(map[int]*Player),
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

// GetNewPlayerID ...
func (g *GameState) GetNewPlayerID() int {
	return int(atomic.AddInt64(&g.PlayerCount, 1))
}

// AddPlayer to the game
func (g *GameState) AddPlayer(player *Player) {
	// g.mutex.Lock()
	g.Players[player.ID] = player
	// g.mutex.Unlock()
}

// RemovePlayer from the game
func (g *GameState) RemovePlayer(player *Player) {
	// g.mutex.Lock()
	delete(g.Players, player.ID)
	// g.mutex.Unlock()
}
