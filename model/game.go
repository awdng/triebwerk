package model

// Game ...
type Game struct {
	playerCount uint8
	Players     map[uint8]*Player
}
