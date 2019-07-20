package model

// Projectile ...
type Projectile struct {
	Position  *Point
	Direction *Point
	Cleanup   bool
}

// ApplyMovement ...
func (b *Projectile) ApplyMovement(dt float32) {
	b.Position.X += b.Direction.X * 100 * dt
	b.Position.Y += b.Direction.Y * 100 * dt
}

// IsCollidingWithPlayer ...
func (b *Projectile) IsCollidingWithPlayer(player *Player) bool {
	enemyPolygon := Polygon{
		Points: []*Point{player.Collider.Rect.A, player.Collider.Rect.B, player.Collider.Rect.C, player.Collider.Rect.D},
	}
	if b.Position.IsInPolygon(enemyPolygon.Points) {
		return true
	}

	return false
}

// IsCollidingWithEnvironment ...
func (b *Projectile) IsCollidingWithEnvironment(m *Map) bool {
	for _, collider := range m.Collider {
		if !collider.Projectile { // projectiles should fly accross this collider
			continue
		}
		if b.Position.IsInPolygon(collider.Points) {
			return true
		}
	}

	return false
}
