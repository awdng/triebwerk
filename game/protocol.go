package game

import (
	"encoding/binary"
	"math"

	"github.com/awdng/triebwerk/model"
)

// encode the current player state to binary
func encode(p *model.Player, currentGameTime uint32, messageType int8) []byte {
	buf := make([]byte, 0)
	buf = append(buf, byte(p.ID))
	buf = append(buf, byte(messageType))

	currentTime := make([]byte, 4)
	binary.LittleEndian.PutUint32(currentTime[:], currentGameTime)
	buf = append(buf, currentTime...)

	switch messageType {
	case 1:
		buf = encodePlayerState(p, buf)
	}

	return buf
}

func encodePlayerState(p *model.Player, buf []byte) []byte {
	posX := make([]byte, 4)
	posY := make([]byte, 4)
	lookX := make([]byte, 4)
	lookY := make([]byte, 4)
	rotation := make([]byte, 4)
	turretRotation := make([]byte, 4)

	binary.LittleEndian.PutUint32(posX[:], math.Float32bits(p.Collider.Pivot.X))
	binary.LittleEndian.PutUint32(posY[:], math.Float32bits(p.Collider.Pivot.Y))
	binary.LittleEndian.PutUint32(lookX[:], math.Float32bits(p.Collider.Look.X))
	binary.LittleEndian.PutUint32(lookY[:], math.Float32bits(p.Collider.Look.Y))
	binary.LittleEndian.PutUint32(rotation[:], math.Float32bits(p.Collider.Rotation))
	binary.LittleEndian.PutUint32(turretRotation[:], math.Float32bits(p.Collider.TurretRotation))

	buf = append(buf, posX...)
	buf = append(buf, posY...)
	buf = append(buf, lookX...)
	buf = append(buf, lookY...)
	buf = append(buf, rotation...)
	buf = append(buf, turretRotation...)

	return buf
}

func decode(data []byte, p *model.Player) {
	p.ID = int(data[0])
	messageType := uint8(data[1])

	switch messageType {
	case 1:
		decodePlayerInput(data, p)
	}
}

func decodePlayerInput(data []byte, p *model.Player) {
	if uint8(data[2]) > 0 {
		p.Control.Forward = true
	} else {
		p.Control.Forward = false
	}
	if uint8(data[3]) > 0 {
		p.Control.Backward = true
	} else {
		p.Control.Backward = false
	}
	if uint8(data[4]) > 0 {
		p.Control.Left = true
	} else {
		p.Control.Left = false
	}
	if uint8(data[5]) > 0 {
		p.Control.Right = true
	} else {
		p.Control.Right = false
	}
	if uint8(data[6]) > 0 {
		p.Control.TurretRight = true
	} else {
		p.Control.TurretRight = false
	}
	if uint8(data[7]) > 0 {
		p.Control.TurretLeft = true
	} else {
		p.Control.TurretLeft = false
	}
	if uint8(data[8]) > 0 {
		p.Control.Shoot = true
	} else {
		p.Control.Shoot = false
	}
}
