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
	ID       int
	Health   int
	Bullets  []*Bullet
	Control  Controls
	Collider *RectCollider
	Client   *Client
}

// Shooting ...
func (p *Player) Shooting(controls Controls, players []*Player, m *Map, dt float32) {
	// move projectiles
	for _, b := range p.Bullets {
		b.Position.X += b.Direction.X * 100 * dt
		b.Position.Y += b.Direction.Y * 100 * dt

		// check bullet collision
		for _, enemy := range players {
			if p.ID == enemy.ID {
				continue
			}
			enemyPolygon := Polygon{
				Points: []*Point{enemy.Collider.Rect.A, enemy.Collider.Rect.B, enemy.Collider.Rect.C, enemy.Collider.Rect.D},
			}
			if b.Position.IsInPolygon(enemyPolygon.Points) {
				fmt.Println("OMG HIT!")
			}
		}
	}

	// create new projectile
	if controls.Shoot {
		bullet := &Bullet{
			Position: &Point{
				X: p.Collider.Turret.X,
				Y: p.Collider.Turret.Y,
			},
		}
		bullet.Direction = bullet.Position.directionTo(p.Collider.Pivot)
		p.Bullets = append(p.Bullets, bullet)
	}
}

// ApplyMovement applies the movement input
func (p *Player) ApplyMovement(controls Controls, players []*Player, m *Map, dt float32) {
	r := p.Collider

	//check collision of this player against other players
	r.CollisionFront = false
	r.CollisionBack = false
	for _, enemy := range players {
		if p.ID == enemy.ID {
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

	if controls.Right {
		r.Rotation += 1.5 * dt
		r.TurretRotation += 1.5 * dt
	}

	if controls.Left {
		r.Rotation -= 1.5 * dt
		r.TurretRotation -= 1.5 * dt
	}

	if controls.TurretRight {
		r.TurretRotation -= 1.5 * dt
	}

	if controls.TurretLeft {
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
	if controls.Forward && !r.CollisionFront {
		movement = 1
		r.Velocity += 15 * dt
	}
	if controls.Backward && !r.CollisionBack {
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

	p.Control = controls
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
