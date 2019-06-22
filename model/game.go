package model

import "time"

// GameState ...
type GameState struct {
	StartTime   time.Time
	PlayerCount uint8
	Players     map[uint8]*Player
}

// GameTime returns the current game time since start in milliseconds
func (g *GameState) GameTime() uint32 {
	return uint32(time.Now().Sub(g.StartTime) / time.Millisecond)
}
