package game

import "github.com/awdng/triebwerk/model"

const width = 5
const depth = 7

// PlayerManager ...
type PlayerManager struct {
}

// NewPlayerManager ...
func NewPlayerManager() *PlayerManager {
	return &PlayerManager{}
}

// NewPlayer creates a new player object
func (p *PlayerManager) NewPlayer(id int, x float32, y float32) *model.Player {
	return &model.Player{
		ID:         id,
		NetworkOut: make(chan []byte),
		Collider:   model.NewRectCollider(x, y, width, depth),
	}
}
