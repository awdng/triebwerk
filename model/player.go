package model

import "time"

const width = 5
const depth = 7

// Connection represents the network connection of the player
type Connection interface {
	Close(writeWait time.Duration, graceful bool)
	Write(writeWait time.Duration, data []byte) error
	Ping(writeWait time.Duration)
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
	collider   RectCollider
	NetworkOut chan []byte
	Connection Connection
}

// Disconnect player from the network
func (p *Player) Disconnect() {
	close(p.NetworkOut)
}
