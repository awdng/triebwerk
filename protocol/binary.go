package protocol

import (
	"encoding/binary"
	"math"
	"strings"
	"unicode/utf8"

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
	protocol.encodeHandlers[8] = encodeScoreboard
	protocol.encodeHandlers[9] = encodeKill

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

// minimum message length per message type, shorter messages are ignored
var minMessageLength = map[uint8]int{
	0: 2,
	1: 13,
	5: 6,
}

// Decode player inputs. Malformed messages are returned without a Body and
// with MessageType 255 so that the game loop ignores them.
func (b BinaryProtocol) Decode(data []byte) model.NetworkMessage {
	message := model.NetworkMessage{
		MessageType: 255,
	}
	if len(data) < 2 {
		return message
	}
	messageType := uint8(data[1])
	if len(data) < minMessageLength[messageType] {
		return message
	}
	message.MessageType = messageType

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
	if p.Fired {
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

// names on the scoreboard are cut to this many bytes
const maxNameLength = 24

// Scoreboard, after the header, numbers little endian:
//
//	uint32  length of a round in ms
//	uint8   number of players, then per player
//	  uint8   id
//	  uint16  kills
//	  uint16  deaths
//	  uint8   length of the name in bytes, followed by the name as utf8
//
// The client's copy of this layout is in src/game/net/protocol.js.
func encodeScoreboard(message *model.NetworkMessage) []byte {
	board := message.Body.(*model.Scoreboard)
	players := board.Players
	if len(players) > math.MaxUint8 {
		players = players[:math.MaxUint8]
	}
	buf := make([]byte, 0, 5+len(players)*(6+maxNameLength))
	buf = binary.LittleEndian.AppendUint32(buf, uint32(board.RoundLength.Milliseconds()))
	buf = append(buf, byte(len(players)))
	for _, p := range players {
		name := scoreboardName(p.Name)
		buf = append(buf, byte(p.ID))
		buf = binary.LittleEndian.AppendUint16(buf, clampUint16(p.Kills))
		buf = binary.LittleEndian.AppendUint16(buf, clampUint16(p.Deaths))
		buf = append(buf, byte(len(name)))
		buf = append(buf, name...)
	}
	return buf
}

// Kill, after the header: uint8 id of the killer, uint8 id of the victim
func encodeKill(message *model.NetworkMessage) []byte {
	kill := message.Body.(model.Kill)
	return []byte{byte(kill.Killer), byte(kill.Victim)}
}

// scoreboardName makes a nickname safe to send: valid utf8 of at most
// maxNameLength bytes, cut between characters
func scoreboardName(name string) string {
	name = strings.ToValidUTF8(name, "")
	for len(name) > maxNameLength {
		_, size := utf8.DecodeLastRuneInString(name)
		name = name[:len(name)-size]
	}
	return name
}

func clampUint16(value int) uint16 {
	if value < 0 {
		return 0
	}
	if value > math.MaxUint16 {
		return math.MaxUint16
	}
	return uint16(value)
}
