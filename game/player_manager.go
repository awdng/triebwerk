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
func (p *PlayerManager) NewPlayer(id uint8, x float32, y float32, conn model.Connection) *model.Player {
	return &model.Player{
		ID:       id,
		Collider: model.NewRectCollider(x, y, width, depth),
		Client: &model.Client{
			NetworkOut: make(chan []byte, 100),
			NetworkIn:  make(chan model.NetworkMessage, 100),
			Connection: conn,
		},
	}
}
