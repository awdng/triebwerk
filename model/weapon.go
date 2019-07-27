package model

const readyTime = 1.2

// Weapon ...
type Weapon struct {
	Projectiles    []*Projectile
	owner          *Player
	ready          bool
	readyCountdown float32
}

// NewWeapon ...
func NewWeapon(owner *Player) *Weapon {
	return &Weapon{
		Projectiles: make([]*Projectile, 0),
		owner:       owner,
		ready:       true,
	}
}

// Update ...
func (w *Weapon) Update(players []*Player, m *Map, dt float32) {
	for _, b := range w.Projectiles {
		b.ApplyMovement(dt)
		// check projectile collision
		// projectile can only hit once
		for _, enemy := range players {
			if w.owner.ID == enemy.ID || !enemy.IsAlive() {
				continue
			}
			if b.IsCollidingWithPlayer(enemy) {
				enemy.Health -= 25
				if enemy.Health <= 0 {
					enemy.Health = 0
					w.owner.Score++
				}
				b.Cleanup = true
				break
			}

			if b.IsCollidingWithEnvironment(m) {
				b.Cleanup = true
				break
			}
		}
	}

	// remove projectiles that hit a target
	newProjectiles := make([]*Projectile, 0)
	for _, projectile := range w.Projectiles {
		if !projectile.Cleanup {
			newProjectiles = append(newProjectiles, projectile)
		}
	}
	w.Projectiles = newProjectiles

	if !w.ready {
		w.readyCountdown += dt
		w.owner.Control.Shoot = false
	}
	if w.readyCountdown > readyTime {
		w.ready = true
		w.readyCountdown = 0
	}
}

// ShootAt ...
func (w *Weapon) ShootAt(posX float32, posY float32) {
	if w.ready {
		projectile := &Projectile{
			Position: &Point{
				X: posX,
				Y: posY,
			},
			Cleanup: false,
		}
		projectile.Direction = projectile.Position.DirectionTo(w.owner.Collider.Pivot)
		w.Projectiles = append(w.Projectiles, projectile)
		w.ready = false
	}
}
