package model

import (
	"fmt"
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
	Identifier() string
}

const respawnTime = 3
const width = 5
const depth = 7

// Controls ...
type Controls struct {
	Forward     bool
	Backward    bool
	Left        bool
	Right       bool
	TurretLeft  bool
	TurretRight bool
	Shoot       bool
	Sequence    uint32
}

// Player ...
type Player struct {
	ID               int
	Health           int
	respawnCountdown float32
	Weapons          []*Weapon
	Control          Controls
	Collider         *RectCollider
	Client           *Client
}

// NewPlayer creates a new player object
func NewPlayer(id int, x float32, y float32, conn Connection) *Player {
	player := &Player{
		ID:       id,
		Health:   100,
		Collider: NewRectCollider(x, y, width, depth),
		Client: &Client{
			NetworkOut: make(chan []byte, 100),
			NetworkIn:  make(chan NetworkMessage, 100),
			Connection: conn,
		},
	}
	player.Weapons = []*Weapon{NewWeapon(player)}
	return player
}

// Update Tick for Player
func (p *Player) Update(players []*Player, game *GameState, dt float32) {
	m := game.Map
	p.handleMovement(players, m, dt)
	p.handleWeapons(players, m, dt)

	if p.Health == 0 {
		p.respawnCountdown += dt
	}
}

// HandleRespawn ...
func (p *Player) HandleRespawn(game *GameState) {
	m := game.Map
	if !p.IsAlive() && p.respawnCountdown > respawnTime {
		p.Health = 100
		p.respawnCountdown = 0

		spawn := m.GetRandomSpawn()
		p.Collider.ChangePosition(spawn.X, spawn.Y)
		p.Collider.Rotation = 0
		p.Collider.TurretRotation = 0
	}
}

// Shooting ...
func (p *Player) handleWeapons(players []*Player, m *Map, dt float32) {
	for _, w := range p.Weapons {
		w.Update(players, m, dt)
	}

	// create new projectile
	if p.Control.Shoot {
		p.Weapons[0].ShootAt(p.Collider.Turret.X, p.Collider.Turret.Y)
	}
}

func (p *Player) handleMovement(players []*Player, m *Map, dt float32) {
	r := p.Collider

	//check collision of this player against other players
	r.CollisionFront = false
	r.CollisionBack = false
	for _, enemy := range players {
		if p.ID == enemy.ID || !enemy.IsAlive() {
			continue
		}
		r.collisionFrontRect(*enemy.Collider)
		r.collisionBackRect(*enemy.Collider)

		if r.CollisionFront || r.CollisionBack {
			break
		}
	}

	//check collision of this player against the environment
	if !r.CollisionFront {
		r.collisionFront(m.Collider)
	}
	if !r.CollisionBack {
		r.collisionBack(m.Collider)
	}

	r.Velocity -= float32(15*1.5) * dt
	if r.Velocity < 0 {
		r.Velocity = 0
	}

	if p.Control.Right {
		r.Rotation += 1.5 * dt
		r.TurretRotation += 1.5 * dt
	}

	if p.Control.Left {
		r.Rotation -= 1.5 * dt
		r.TurretRotation -= 1.5 * dt
	}

	if p.Control.TurretRight {
		r.TurretRotation -= 1.5 * dt
	}

	if p.Control.TurretLeft {
		r.TurretRotation += 1.5 * dt
	}

	rotationDelta := r.Rotation - r.LastRotation
	turretRotationDelta := r.TurretRotation - r.TurretLastRotation

	r.rotateRectPoint(rotationDelta, r.Rect.A)
	r.rotateRectPoint(rotationDelta, r.Rect.B)
	r.rotateRectPoint(rotationDelta, r.Rect.C)
	r.rotateRectPoint(rotationDelta, r.Rect.D)

	r.rotateRectPoint(rotationDelta, r.Look)
	r.rotateRectPoint(turretRotationDelta, r.Turret)
	r.CalcDirection()

	movement := 0
	if p.Control.Forward && !r.CollisionFront {
		movement = 1
		r.Velocity += 15 * dt
	}
	if p.Control.Backward && !r.CollisionBack {
		movement = -1
		r.Velocity -= 15 * dt
	}

	if movement != 0 {
		r.Rect.A.X += r.Dir.X * r.Velocity
		r.Rect.A.Y += r.Dir.Y * r.Velocity

		r.Rect.B.X += r.Dir.X * r.Velocity
		r.Rect.B.Y += r.Dir.Y * r.Velocity

		r.Rect.C.X += r.Dir.X * r.Velocity
		r.Rect.C.Y += r.Dir.Y * r.Velocity

		r.Rect.D.X += r.Dir.X * r.Velocity
		r.Rect.D.Y += r.Dir.Y * r.Velocity

		r.Pivot.X += r.Dir.X * r.Velocity
		r.Pivot.Y += r.Dir.Y * r.Velocity

		r.Look.X += r.Dir.X * r.Velocity
		r.Look.Y += r.Dir.Y * r.Velocity

		r.Turret.X += r.Dir.X * r.Velocity
		r.Turret.Y += r.Dir.Y * r.Velocity
	}
	r.LastRotation = r.Rotation
	r.TurretLastRotation = r.TurretRotation
}

// IsAlive ...
func (p *Player) IsAlive() bool {
	if p.Health == 0 {
		return false
	}
	return true
}

// Client represents a network client
type Client struct {
	NetworkOut chan []byte
	NetworkIn  chan NetworkMessage
	Connection Connection
}

// Disconnect Client from the network
func (c *Client) Disconnect() {
	close(c.NetworkOut)
}

// NetworkMessage represents an network message from or to a Client
type NetworkMessage struct {
	MessageType uint8
	Body        interface{}
}

func (m NetworkMessage) String() string {
	return fmt.Sprintf("NetworkMessage %d - %+v", m.MessageType, m.Body)
}
