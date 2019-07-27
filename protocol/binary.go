package protocol

import (
	"encoding/binary"
	"math"

	"github.com/awdng/triebwerk/model"
)

// BinaryProtocol ...
type BinaryProtocol struct {
	encodeHandlers map[uint8]func(message *model.NetworkMessage) []byte
	decodeHandlers map[uint8]func(data []byte, message *model.NetworkMessage)
}

// NewBinaryProtocol ...
func NewBinaryProtocol() BinaryProtocol {
	protocol := BinaryProtocol{
		encodeHandlers: make(map[uint8]func(message *model.NetworkMessage) []byte),
		decodeHandlers: make(map[uint8]func(data []byte, message *model.NetworkMessage)),
	}

	// register Handlers by messageType
	protocol.encodeHandlers[1] = encodePlayerState
	protocol.encodeHandlers[2] = encodePlayerRegister
	protocol.encodeHandlers[5] = encodePlayerTime

	protocol.decodeHandlers[0] = decodePlayerAuth
	protocol.decodeHandlers[1] = decodePlayerInput
	protocol.decodeHandlers[5] = decodePlayerTime

	return protocol
}

// Encode data to send to clients
func (b BinaryProtocol) Encode(id int, currentGameTime uint32, message *model.NetworkMessage) []byte {
	buf := make([]byte, 0)
	buf = append(buf, byte(uint8(id)))
	buf = append(buf, byte(message.MessageType))

	currentTime := make([]byte, 4)
	binary.LittleEndian.PutUint32(currentTime[:], currentGameTime)
	buf = append(buf, currentTime...)

	if encodeHandler, ok := b.encodeHandlers[message.MessageType]; ok {
		buf = append(buf, encodeHandler(message)...)
	}

	return buf
}

// Decode player inputs
func (b BinaryProtocol) Decode(data []byte) model.NetworkMessage {
	message := model.NetworkMessage{
		MessageType: uint8(data[1]),
	}

	if decodeHandler, ok := b.decodeHandlers[message.MessageType]; ok {
		decodeHandler(data, &message)
	}
	return message
}

func encodePlayerState(message *model.NetworkMessage) []byte {
	p := message.Body.(*model.Player)
	buf := make([]byte, 0, 30)
	sequence := make([]byte, 4)
	posX := make([]byte, 4)
	posY := make([]byte, 4)
	turretX := make([]byte, 4)
	turretY := make([]byte, 4)
	rotation := make([]byte, 4)
	turretRotation := make([]byte, 4)
	shooting := 0

	binary.LittleEndian.PutUint32(sequence[:], p.Control.Sequence)
	binary.LittleEndian.PutUint32(posX[:], math.Float32bits(p.Collider.Pivot.X))
	binary.LittleEndian.PutUint32(posY[:], math.Float32bits(p.Collider.Pivot.Y))
	binary.LittleEndian.PutUint32(turretX[:], math.Float32bits(p.Collider.Turret.X))
	binary.LittleEndian.PutUint32(turretY[:], math.Float32bits(p.Collider.Turret.Y))
	binary.LittleEndian.PutUint32(rotation[:], math.Float32bits(p.Collider.Rotation))
	binary.LittleEndian.PutUint32(turretRotation[:], math.Float32bits(p.Collider.TurretRotation))
	if p.Control.Shoot {
		shooting = 1
	}

	buf = append(buf, sequence...)
	buf = append(buf, posX...)
	buf = append(buf, posY...)
	buf = append(buf, turretX...)
	buf = append(buf, turretY...)
	buf = append(buf, rotation...)
	buf = append(buf, turretRotation...)
	buf = append(buf, byte(shooting))
	buf = append(buf, byte(p.Health))

	return buf
}

func encodePlayerRegister(message *model.NetworkMessage) []byte {
	// for now do nothing
	return []byte{}
}

func encodePlayerTime(message *model.NetworkMessage) []byte {
	time := make([]byte, 4)
	binary.LittleEndian.PutUint32(time[:], message.Body.(uint32))

	return time
}

func EncodePlayerInput() {

}

func decodePlayerInput(data []byte, message *model.NetworkMessage) {
	controls := model.Controls{}
	controls.Forward = false
	controls.Backward = false
	controls.Left = false
	controls.Right = false
	controls.TurretRight = false
	controls.TurretLeft = false
	controls.Shoot = false

	if uint8(data[2]) == 1 {
		controls.Forward = true
	}
	if uint8(data[3]) == 1 {
		controls.Backward = true
	}
	if uint8(data[4]) == 1 {
		controls.Left = true
	}
	if uint8(data[5]) == 1 {
		controls.Right = true
	}
	if uint8(data[6]) == 1 {
		controls.TurretRight = true
	}
	if uint8(data[7]) == 1 {
		controls.TurretLeft = true
	}
	if uint8(data[8]) == 1 {
		controls.Shoot = true
	}
	controls.Sequence = binary.BigEndian.Uint32(data[9:])
	message.Body = controls
}

func decodePlayerTime(data []byte, message *model.NetworkMessage) {
	message.Body = binary.BigEndian.Uint32(data[2:])
}

func decodePlayerAuth(data []byte, message *model.NetworkMessage) {
	message.Body = string(data[2:])
}
