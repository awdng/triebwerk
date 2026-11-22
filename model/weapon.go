package model

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

// Reset makes the weapon what it is at the start of a round: loaded, with
// no shell in the air
func (w *Weapon) Reset() {
	w.Projectiles = make([]*Projectile, 0)
	w.ready = true
	w.readyCountdown = 0
}

// Update ...
func (w *Weapon) Update(players []*Player, m *Map, dt float32) {
	for _, b := range w.Projectiles {
		b.ApplyMovement(dt)
		// check projectile collision
		// projectile can only hit once
		if m.IsOutOfBounds(b.Position) || b.IsCollidingWithEnvironment(m) {
			b.Cleanup = true
			continue
		}
		for _, enemy := range players {
			if w.owner.ID == enemy.ID || !enemy.IsAlive() {
				continue
			}
			if b.IsCollidingWithPlayer(enemy) {
				// what the gun gives, as much of it as the armour lets through
				enemy.Health -= w.owner.Spec().DamageTo(enemy.Spec())
				if enemy.Health <= 0 {
					enemy.Health = 0
					enemy.Deaths++
					w.owner.Score++
					w.owner.KillLog = append(w.owner.KillLog, enemy.ID)
				}
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
	// how long this kind of tank takes to load, see TankSpec.ReloadTicks
	if w.readyCountdown > w.owner.Spec().Reload {
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
		w.owner.Fired = true
		w.owner.Shots++
	}
}
