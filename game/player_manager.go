package game

import "github.com/awdng/triebwerk/model"

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
		controls:   make(chan model.Controls, 256),
		collider:   model.NewRectCollider(x, y, width, depth),
	}

}
