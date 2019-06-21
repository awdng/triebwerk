package model

// GameState ...
type GameState struct {
	PlayerCount uint8
	Players     map[uint8]*Player
}
