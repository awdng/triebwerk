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
	ID        int
	GlobalID  string
	AuthToken string
	Nickname  string
	Health    int
	// Score counts the players this one destroyed in the running round
	Score int
	// Deaths counts how often this player was destroyed in the running round
	Deaths int
	// KillLog collects the ids of the players this one destroyed until the
	// game loop reports them with TakeKills
	KillLog []int
	// Fired is true if a projectile was fired during the current server tick
	Fired bool
	// Shots counts the projectiles fired, it wraps. Clients see a shot as a
	// change of this number, also when the tick that fired did not reach them.
	Shots            uint8
	respawnCountdown float32
	Weapons          []*Weapon
	// Control holds the keys for the coming tick, see GameState.ApplyInputs
	Control Controls
	// Inputs are the inputs of the client that wait for their tick
	Inputs *InputQueue
	// Starved is true if the client's input for the current tick did not
	// arrive and the last one was applied again
	Starved  bool
	Collider *RectCollider
	Client   *Client
}

// NewPlayer creates a new player object
func NewPlayer(id int, x float32, y float32, conn Connection) *Player {
	player := &Player{
		ID:       id,
		Health:   100,
		Collider: NewRectCollider(x, y, width, depth),
		Inputs:   NewInputQueue(),
		Client:   NewClient(conn),
	}
	player.Weapons = []*Weapon{NewWeapon(player)}
	return player
}

// HandleRespawn ...
func (p *Player) HandleRespawn(game *GameState) {
	m := game.Map
	if !p.IsAlive() && p.respawnCountdown > respawnTime {
		spawn := m.GetRandomSpawn(game.GetPlayers())
		p.Health = 100
		p.respawnCountdown = 0

		p.Collider.ChangePosition(spawn.X, spawn.Y)
		p.Collider.Rotation = 0
		p.Collider.TurretRotation = 0
	}
}

// HandleMovement ...
func (p *Player) HandleMovement(players []*Player, m *Map, dt float32) {
	r := p.Collider

	//check collision of this player against other players
	r.CollisionFront = false
	r.CollisionBack = false
	for _, enemy := range players {
		if p.ID == enemy.ID || !enemy.IsAlive() {
			continue
		}
		if r.collisionPolygon(enemy.Collider.getPolygon()) { // simple check if polygons intersect
			r.collisionFrontRect(*enemy.Collider)
			if r.CollisionFront {
				break
			}

			r.collisionBackRect(*enemy.Collider)
			if r.CollisionBack {
				break
			}
		}
	}

	//check collision of this player against the environment
	if !r.CollisionFront && !r.CollisionBack { // only if not already colliding with player
		for _, collider := range m.Collider {
			if r.collisionPolygon(collider.getPolygon()) { // simple check if polygons intersect
				// check if collision occured front or back
				r.collisionFront(collider.getPolygon())
				if r.CollisionFront {
					break
				}
				r.collisionBack(collider.getPolygon())
				if r.CollisionBack {
					break
				}
			}
		}
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

	// the turret turns the way the hull does for the keys of the same name
	if p.Control.TurretRight {
		r.TurretRotation += 1.5 * dt
	}

	if p.Control.TurretLeft {
		r.TurretRotation -= 1.5 * dt
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

// WaitingStarved is what InputsWaiting reports for a tick without an input
const WaitingStarved = 255

// InputsWaiting tells a client how its inputs arrive: how many of them wait
// for their tick, or WaitingStarved if the current tick had none. A client
// keeps this near one by sending a little faster or slower.
func (p *Player) InputsWaiting() uint8 {
	if p.Starved {
		return WaitingStarved
	}
	if waiting := p.Inputs.Waiting(); waiting < WaitingStarved {
		return uint8(waiting)
	}
	return WaitingStarved - 1
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

// NewClient wraps a connection that just arrived
func NewClient(conn Connection) *Client {
	return &Client{
		NetworkOut: make(chan []byte, 100),
		NetworkIn:  make(chan NetworkMessage, 100),
		Connection: conn,
	}
}

// Disconnect Client from the network
// NetworkIn is not closed because the connection reader might still be writing to it.
func (c *Client) Disconnect() {
	close(c.NetworkOut)
}

// Hello is the first message of a client: the version of the wire format it
// speaks and the token the master server knows it by
type Hello struct {
	Version uint8
	Token   string
}

// Ping is answered with the same number, clients measure the round trip with it
type Ping struct {
	ClientTime uint32
}

// NetworkMessage represents an network message from or to a Client
type NetworkMessage struct {
	MessageType uint8
	Body        interface{}
}

func (m NetworkMessage) String() string {
	return fmt.Sprintf("NetworkMessage %d - %+v", m.MessageType, m.Body)
}

// TakeKills returns the ids of the players destroyed since the last call
func (p *Player) TakeKills() []int {
	kills := p.KillLog
	p.KillLog = nil
	return kills
}
