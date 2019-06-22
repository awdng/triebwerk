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
	ID       uint8
	Control  Controls
	Collider RectCollider
	Client   *Client
}

// Client represents a network client
type Client struct {
	NetworkOut chan []byte
	Connection Connection
}

// Disconnect Client from the network
func (c *Client) Disconnect() {
	close(c.NetworkOut)
}
