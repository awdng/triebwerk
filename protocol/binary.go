package protocol

import (
	"encoding/binary"
	"math"

	"github.com/awdng/triebwerk/model"
)

// BinaryProtocol ...
type BinaryProtocol struct {
	encodeHandlers map[int8]func(p *model.Player, buf []byte) []byte
	decodeHandlers map[int8]func(data []byte, p *model.Player)
}

func NewBinaryProtocol() BinaryProtocol {
	protocol := BinaryProtocol{
		encodeHandlers: make(map[int8]func(p *model.Player, buf []byte) []byte),
		decodeHandlers: make(map[int8]func(data []byte, p *model.Player)),
	}

	// register Handlers by messageType
	protocol.encodeHandlers[1] = encodePlayerState
	protocol.decodeHandlers[1] = decodePlayerInput

	return protocol
}

// Encode the current player state
func (b BinaryProtocol) Encode(p *model.Player, currentGameTime uint32, messageType int8) []byte {
	buf := make([]byte, 0)
	buf = append(buf, byte(p.ID))
	buf = append(buf, byte(messageType))

	currentTime := make([]byte, 4)
	binary.LittleEndian.PutUint32(currentTime[:], currentGameTime)
	buf = append(buf, currentTime...)

	if encodeHandler, ok := b.encodeHandlers[messageType]; ok {
		encodeHandler(p, buf)
	}

	return buf
}

// Decode player inputs
func (b BinaryProtocol) Decode(data []byte, p *model.Player) {
	p.ID = int(data[0])
	messageType := int8(data[1])

	if decodeHandler, ok := b.decodeHandlers[messageType]; ok {
		decodeHandler(data, p)
	}
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

func decodePlayerInput(data []byte, p *model.Player) {
	p.Control.Forward = false
	p.Control.Backward = false
	p.Control.Left = false
	p.Control.Right = false
	p.Control.TurretRight = false
	p.Control.TurretLeft = false
	p.Control.Shoot = false

	if uint8(data[2]) == 1 {
		p.Control.Forward = true
	}
	if uint8(data[3]) == 1 {
		p.Control.Backward = true
	}
	if uint8(data[4]) == 1 {
		p.Control.Left = true
	}
	if uint8(data[5]) == 1 {
		p.Control.Right = true
	}
	if uint8(data[6]) == 1 {
		p.Control.TurretRight = true
	}
	if uint8(data[7]) == 1 {
		p.Control.TurretLeft = true
	}
	if uint8(data[8]) == 1 {
		p.Control.Shoot = true
	}
}
