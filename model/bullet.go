package model

// Bullet ...
type Bullet struct {
	Position  *Point
	Direction *Point
}

// ApplyMovement ...
func (b *Bullet) ApplyMovement(dt float32) {
	b.Position.X += b.Direction.X * 100 * dt
	b.Position.Y += b.Direction.Y * 100 * dt
}

// IsCollidingWithPlayer ...
func (b *Bullet) IsCollidingWithPlayer(player *Player) bool {
	enemyPolygon := Polygon{
		Points: []*Point{player.Collider.Rect.A, player.Collider.Rect.B, player.Collider.Rect.C, player.Collider.Rect.D},
	}
	if b.Position.IsInPolygon(enemyPolygon.Points) {
		return true
	}

	return false
}
