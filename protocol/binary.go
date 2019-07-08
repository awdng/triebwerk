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

	protocol.decodeHandlers[1] = decodePlayerInput
	protocol.decodeHandlers[5] = decodePlayerTime

	return protocol
}

// Encode data to send to clients
func (b BinaryProtocol) Encode(id uint8, currentGameTime uint32, message *model.NetworkMessage) []byte {
	buf := make([]byte, 0)
	buf = append(buf, byte(id))
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
	buf := make([]byte, 0, 28)
	posX := make([]byte, 4)
	posY := make([]byte, 4)
	lookX := make([]byte, 4)
	lookY := make([]byte, 4)
	rotation := make([]byte, 4)
	turretRotation := make([]byte, 4)
	sequence := make([]byte, 4)

	binary.LittleEndian.PutUint32(posX[:], math.Float32bits(p.Collider.Pivot.X))
	binary.LittleEndian.PutUint32(posY[:], math.Float32bits(p.Collider.Pivot.Y))
	binary.LittleEndian.PutUint32(lookX[:], math.Float32bits(p.Collider.Look.X))
	binary.LittleEndian.PutUint32(lookY[:], math.Float32bits(p.Collider.Look.Y))
	binary.LittleEndian.PutUint32(rotation[:], math.Float32bits(p.Collider.Rotation))
	binary.LittleEndian.PutUint32(turretRotation[:], math.Float32bits(p.Collider.TurretRotation))
	binary.LittleEndian.PutUint32(sequence[:], p.Control.Sequence)

	buf = append(buf, posX...)
	buf = append(buf, posY...)
	buf = append(buf, lookX...)
	buf = append(buf, lookY...)
	buf = append(buf, rotation...)
	buf = append(buf, turretRotation...)
	buf = append(buf, sequence...)

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
