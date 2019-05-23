package model

import (
	"time"
)

// Connection represents the network connection of the player
type Connection interface {
	Ping(writeWait time.Duration)
	Close(writeWait time.Duration, graceful bool)
	PrepareWrite(writeWait time.Duration)
	Write(data []byte) error
	PrepareRead(maxMessageSize int64, pongWait time.Duration)
	Read() ([]byte, error)
}

// Controls ...
type Controls struct {
	Forward     bool
	Backward    bool
	Left        bool
	Right       bool
	TurretLeft  bool
	TurretRight bool
	Shoot       bool
}

// Player ...
type Player struct {
	ID         int
	Control    Controls
	Collider   RectCollider
	NetworkOut chan []byte
	Connection Connection
}

// Disconnect player from the network
func (p *Player) Disconnect() {
	close(p.NetworkOut)
}
