package game

import "time"

// Game represents the game state
type Game struct {
	tickStart time.Time
	startTime time.Time
}

// NewGame creates a game instance
func NewGame() *Game {
	return &Game{
		startTime: time.Now(),
	}
}

// GameTime returns the current game time since start in milliseconds
// TODO: review return type
func (g *Game) GameTime() uint32 {
	return uint32(time.Now().Sub(g.startTime) / time.Millisecond)
}
