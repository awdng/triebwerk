// Package protocol is the wire format between the game server and its
// clients, version 2. The client's copy is src/game/net/protocol.js; the two
// change together.
//
// A message is one byte of type followed by its fields, numbers little
// endian in both directions. There are two lanes:
//
// The reliable lane carries what must arrive, in order (types below 16):
//
//	1  Hello       c>s  version u8, auth token (utf8, to the end of the message)
//	2  Welcome     s>c  version u8, player id u8, tick length in ms u8,
//	                    round length in ms u32, current tick u32
//	3  Reject      s>c  version u8 the server speaks; the connection is closed
//	4  RoundStart  s>c  tick u32 the round started at
//	5  RoundEnd    s>c  tick u32 the round ended at
//	6  Scoreboard  s>c  round length in ms u32, count u8, then per player:
//	                    id u8, kills u16, deaths u16, name length u8, name utf8
//	7  Kill        s>c  killer id u8, victim id u8
//	8  Denied      s>c  reason u8: 1 the master server does not accept the
//	                    token (sign in again), 2 the master server did not
//	                    answer, 3 the account is in a match on another
//	                    server, 4 the account joined this server again and
//	                    that connection plays now; the connection is closed
//	9  Accounts    s>c  count u8, then that many player ids u8: who on the
//	                    scoreboard plays with an account of the master
//	                    server, everybody else is a guest. Follows every
//	                    scoreboard.
//	10 Loadout     c>s  kind of tank u8 (model.Tanks), skin u8: what the
//	                    player wants to drive. The first one of a connection
//	                    counts at once while the tank has not been driven
//	                    (clients send it right behind the hello), every
//	                    other one at the next spawn. Who sends none drives
//	                    the first kind.
//	11 Loadouts    s>c  count u8, then per player: id u8, kind of tank u8,
//	                    skin u8. Follows every scoreboard, after Accounts.
//
// The unreliable lane carries what is replaced by the next message anyway
// (types from 16). Messages may be lost, doubled or arrive out of order:
//
//	16 Input       c>s  number of the newest input u32, count u8 (1..3), then
//	                    one byte of keys per input, newest first: the inputs
//	                    before the newest ride along in case their own message
//	                    was lost
//	17 Snapshot    s>c  tick u32, number of the receiver's last input that
//	                    went into this state u32 (a tick that was bridged
//	                    because its input was late counts as that input, see
//	                    model.InputQueue), inputs of the receiver that wait u8
//	                    (255: this tick had none), count u8, then per tank:
//	                    id u8, x f32, y f32, rotation f32, turret rotation f32,
//	                    health u8, shots fired u8 (a counter that wraps)
//	18 Ping        c>s  client time u32
//	19 Pong        s>c  the client time of the ping u32
//
// Over a WebSocket both lanes share the socket. Over WebTransport the
// reliable lane is a stream and the unreliable lane datagrams.
package protocol

import (
	"encoding/binary"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/awdng/triebwerk/model"
)

// Version of the wire format. A client that says another one is rejected.
const Version = 2

// message types
const (
	Hello      = 1
	Welcome    = 2
	Reject     = 3
	Denied     = 8
	Accounts   = 9
	Loadout    = 10
	Loadouts   = 11
	RoundStart = 4
	RoundEnd   = 5
	Scoreboard = 6
	Kill       = 7

	Input    = 16
	Snapshot = 17
	Ping     = 18
	Pong     = 19

	// Malformed is the type of a message that could not be read
	Malformed = 255
)

// keys of an input
const (
	BitForward     = 1
	BitBackward    = 2
	BitLeft        = 4
	BitRight       = 8
	BitTurretLeft  = 16
	BitTurretRight = 32
	BitShoot       = 64
)

const (
	// MaxInputsPerMessage is how many inputs one message carries at most
	MaxInputsPerMessage = 3
	// WaitingStarved in a snapshot: the receiver's tank got no input this tick
	WaitingStarved = model.WaitingStarved
	// SnapshotEntrySize is the size of one tank in a snapshot
	SnapshotEntrySize = 19
	snapshotHeader    = 11
	// A snapshot has to fit into one datagram. A QUIC packet carries about
	// 1200 bytes, some of them are its own.
	maxDatagram = 1100
	// MaxSnapshotPlayers is how many tanks fit into one snapshot
	MaxSnapshotPlayers = (maxDatagram - snapshotHeader) / SnapshotEntrySize
	// Names on the scoreboard are cut to this many bytes. The longest name
	// the master server gives fits whole: 20 characters of up to four bytes
	// and " (guest)". A name that was cut could be taken for another.
	maxNameLength = 88
)

// IsReliable says which lane a message type travels on
func IsReliable(messageType byte) bool {
	return messageType < Input
}

// BinaryProtocol encodes and decodes the messages of the game
type BinaryProtocol struct{}

// NewBinaryProtocol ...
func NewBinaryProtocol() BinaryProtocol {
	return BinaryProtocol{}
}

// Version a client has to speak
func (BinaryProtocol) Version() uint8 {
	return Version
}

// IsReliable says which lane a message type travels on
func (BinaryProtocol) IsReliable(messageType byte) bool {
	return IsReliable(messageType)
}

// ---- from clients -----------------------------------------------------------

// Decode reads a message of a client. What can not be read comes back as
// type Malformed without a body, the game ignores it.
func (BinaryProtocol) Decode(data []byte) model.NetworkMessage {
	message := model.NetworkMessage{MessageType: Malformed}
	if len(data) == 0 {
		return message
	}
	switch data[0] {
	case Hello:
		if len(data) < 2 {
			return message
		}
		message.Body = model.Hello{Version: data[1], Token: strings.ToValidUTF8(string(data[2:]), "")}
	case Input:
		inputs := decodeInputs(data)
		if len(inputs) == 0 {
			return message
		}
		message.Body = inputs
	case Ping:
		if len(data) < 5 {
			return message
		}
		message.Body = model.Ping{ClientTime: binary.LittleEndian.Uint32(data[1:])}
	case Loadout:
		if len(data) != 3 {
			return message
		}
		message.Body = model.Loadout{Tank: data[1], Skin: data[2]}
	default:
		return message
	}
	message.MessageType = data[0]
	return message
}

// decodeInputs returns the inputs of a message, newest first. A message
// that announces more inputs than it carries or than are allowed gives the
// ones that are there.
func decodeInputs(data []byte) []model.Controls {
	if len(data) < 7 {
		return nil
	}
	newest := binary.LittleEndian.Uint32(data[1:])
	count := int(data[5])
	if count > MaxInputsPerMessage {
		count = MaxInputsPerMessage
	}
	if count > len(data)-6 {
		count = len(data) - 6
	}
	inputs := make([]model.Controls, 0, count)
	for i := 0; i < count; i++ {
		// inputs are numbered from 1
		if uint32(i) >= newest {
			break
		}
		keys := data[6+i]
		inputs = append(inputs, model.Controls{
			Sequence:    newest - uint32(i),
			Forward:     keys&BitForward != 0,
			Backward:    keys&BitBackward != 0,
			Left:        keys&BitLeft != 0,
			Right:       keys&BitRight != 0,
			TurretLeft:  keys&BitTurretLeft != 0,
			TurretRight: keys&BitTurretRight != 0,
			Shoot:       keys&BitShoot != 0,
		})
	}
	return inputs
}

// ---- to clients -------------------------------------------------------------

// Welcome tells a client its player id and how this server keeps time
func (BinaryProtocol) Welcome(id int, tick uint32, roundLength time.Duration) []byte {
	buf := []byte{Welcome, Version, byte(id), model.TickMillis}
	buf = binary.LittleEndian.AppendUint32(buf, uint32(roundLength.Milliseconds()))
	return binary.LittleEndian.AppendUint32(buf, tick)
}

// Reject turns away a client that speaks another version
func (BinaryProtocol) Reject() []byte {
	return []byte{Reject, Version}
}

// Why a player is turned away after its hello
const (
	DeniedNotAuthorized = model.DeniedNotAuthorized
	DeniedNoMaster      = model.DeniedNoMaster
	DeniedElsewhere     = model.DeniedElsewhere
	DeniedReplaced      = model.DeniedReplaced
)

// Denied turns away a player the master server did not vouch for
func (BinaryProtocol) Denied(reason byte) []byte {
	return []byte{Denied, reason}
}

// RoundStart ...
func (BinaryProtocol) RoundStart(tick uint32) []byte {
	return binary.LittleEndian.AppendUint32([]byte{RoundStart}, tick)
}

// RoundEnd ...
func (BinaryProtocol) RoundEnd(tick uint32) []byte {
	return binary.LittleEndian.AppendUint32([]byte{RoundEnd}, tick)
}

// Kill says who destroyed whom
func (BinaryProtocol) Kill(kill model.Kill) []byte {
	return []byte{Kill, byte(kill.Killer), byte(kill.Victim)}
}

// Pong answers a ping with its own number
func (BinaryProtocol) Pong(clientTime uint32) []byte {
	return binary.LittleEndian.AppendUint32([]byte{Pong}, clientTime)
}

// SnapshotBody is the part of a snapshot that is the same for every client:
// the number of tanks and the tanks
func (BinaryProtocol) SnapshotBody(players []*model.Player) []byte {
	if len(players) > MaxSnapshotPlayers {
		players = players[:MaxSnapshotPlayers]
	}
	buf := make([]byte, 0, 1+len(players)*SnapshotEntrySize)
	buf = append(buf, byte(len(players)))
	for _, p := range players {
		buf = append(buf, byte(p.ID))
		buf = binary.LittleEndian.AppendUint32(buf, math.Float32bits(p.Collider.Pivot.X))
		buf = binary.LittleEndian.AppendUint32(buf, math.Float32bits(p.Collider.Pivot.Y))
		buf = binary.LittleEndian.AppendUint32(buf, math.Float32bits(p.Collider.Rotation))
		buf = binary.LittleEndian.AppendUint32(buf, math.Float32bits(p.Collider.TurretRotation))
		buf = append(buf, byte(p.Health), p.Shots)
	}
	return buf
}

// Snapshot puts the receiver's own header before a body: the tick the state
// belongs to, the last input of the receiver that went into it and how many
// of its inputs wait
func (BinaryProtocol) Snapshot(tick uint32, ack uint32, waiting uint8, body []byte) []byte {
	buf := make([]byte, 0, snapshotHeader-1+len(body))
	buf = append(buf, Snapshot)
	buf = binary.LittleEndian.AppendUint32(buf, tick)
	buf = binary.LittleEndian.AppendUint32(buf, ack)
	buf = append(buf, waiting)
	return append(buf, body...)
}

// Scoreboard: names, kills and deaths of everybody
func (BinaryProtocol) Scoreboard(board *model.Scoreboard) []byte {
	players := board.Players
	if len(players) > math.MaxUint8 {
		players = players[:math.MaxUint8]
	}
	buf := make([]byte, 0, 6+len(players)*(6+maxNameLength))
	buf = append(buf, Scoreboard)
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

// Accounts: the ids of the players on a scoreboard who have an account
func (BinaryProtocol) Accounts(board *model.Scoreboard) []byte {
	buf := []byte{Accounts, 0}
	for _, p := range board.Players {
		if p.Account && len(buf) < 2+math.MaxUint8 {
			buf = append(buf, byte(p.ID))
		}
	}
	buf[1] = byte(len(buf) - 2)
	return buf
}

// Loadouts: what everybody on a scoreboard drives
func (BinaryProtocol) Loadouts(board *model.Scoreboard) []byte {
	players := board.Players
	if len(players) > math.MaxUint8 {
		players = players[:math.MaxUint8]
	}
	buf := make([]byte, 0, 2+len(players)*3)
	buf = append(buf, Loadouts, byte(len(players)))
	for _, p := range players {
		buf = append(buf, byte(p.ID), p.Loadout.Tank, p.Loadout.Skin)
	}
	return buf
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

// ---- the client's side, for bots and tests ------------------------------------

// EncodeHello is the first message of a client
func EncodeHello(token string) []byte {
	return append([]byte{Hello, Version}, token...)
}

// EncodeLoadout is what a client says it wants to drive
func EncodeLoadout(loadout model.Loadout) []byte {
	return []byte{Loadout, loadout.Tank, loadout.Skin}
}

// EncodeInput carries the keys of the newest input and of the ones before
// it, newest first
func EncodeInput(newest uint32, keys []byte) []byte {
	if len(keys) > MaxInputsPerMessage {
		keys = keys[:MaxInputsPerMessage]
	}
	buf := binary.LittleEndian.AppendUint32([]byte{Input}, newest)
	buf = append(buf, byte(len(keys)))
	return append(buf, keys...)
}

// EncodePing ...
func EncodePing(clientTime uint32) []byte {
	return binary.LittleEndian.AppendUint32([]byte{Ping}, clientTime)
}

// SnapshotEntry is one tank of a snapshot
type SnapshotEntry struct {
	ID             int
	X, Y           float32
	Rotation       float32
	TurretRotation float32
	Health         int
	Shots          uint8
}

// SnapshotMessage is a snapshot as a client reads it
type SnapshotMessage struct {
	Tick    uint32
	Ack     uint32
	Waiting uint8
	Players []SnapshotEntry
}

// DecodeSnapshot reads a snapshot. It is strict: every byte has to be
// accounted for.
func DecodeSnapshot(data []byte) (SnapshotMessage, bool) {
	var snapshot SnapshotMessage
	if len(data) < snapshotHeader || data[0] != Snapshot {
		return snapshot, false
	}
	count := int(data[10])
	if len(data) != snapshotHeader+count*SnapshotEntrySize {
		return snapshot, false
	}
	snapshot.Tick = binary.LittleEndian.Uint32(data[1:])
	snapshot.Ack = binary.LittleEndian.Uint32(data[5:])
	snapshot.Waiting = data[9]
	snapshot.Players = make([]SnapshotEntry, count)
	float := func(offset int) float32 { return math.Float32frombits(binary.LittleEndian.Uint32(data[offset:])) }
	for i := range snapshot.Players {
		at := snapshotHeader + i*SnapshotEntrySize
		snapshot.Players[i] = SnapshotEntry{
			ID:             int(data[at]),
			X:              float(at + 1),
			Y:              float(at + 5),
			Rotation:       float(at + 9),
			TurretRotation: float(at + 13),
			Health:         int(data[at+17]),
			Shots:          data[at+18],
		}
	}
	return snapshot, true
}

// DecodeLoadouts reads what everybody drives, by player id. It is strict,
// like DecodeScoreboard.
func DecodeLoadouts(data []byte) (map[int]model.Loadout, bool) {
	if len(data) < 2 || data[0] != Loadouts || len(data) != 2+3*int(data[1]) {
		return nil, false
	}
	loadouts := make(map[int]model.Loadout, data[1])
	for at := 2; at < len(data); at += 3 {
		loadouts[int(data[at])] = model.Loadout{Tank: data[at+1], Skin: data[at+2]}
	}
	return loadouts, true
}

// DecodeAccounts reads who on the scoreboard has an account. It is strict,
// like DecodeScoreboard.
func DecodeAccounts(data []byte) ([]int, bool) {
	if len(data) < 2 || data[0] != Accounts || len(data) != 2+int(data[1]) {
		return nil, false
	}
	ids := make([]int, 0, len(data)-2)
	for _, id := range data[2:] {
		ids = append(ids, int(id))
	}
	return ids, true
}

// DecodeScoreboard reads a scoreboard. It is strict: every byte has to be
// accounted for and names have to be utf8.
func DecodeScoreboard(data []byte) (*model.Scoreboard, bool) {
	if len(data) < 6 || data[0] != Scoreboard {
		return nil, false
	}
	board := &model.Scoreboard{RoundLength: time.Duration(binary.LittleEndian.Uint32(data[1:])) * time.Millisecond}
	count := int(data[5])
	offset := 6
	for i := 0; i < count; i++ {
		if offset+6 > len(data) {
			return nil, false
		}
		nameLength := int(data[offset+5])
		if offset+6+nameLength > len(data) {
			return nil, false
		}
		name := data[offset+6 : offset+6+nameLength]
		if !utf8.Valid(name) {
			return nil, false
		}
		board.Players = append(board.Players, model.ScoreboardEntry{
			ID:     int(data[offset]),
			Kills:  int(binary.LittleEndian.Uint16(data[offset+1:])),
			Deaths: int(binary.LittleEndian.Uint16(data[offset+3:])),
			Name:   string(name),
		})
		offset += 6 + nameLength
	}
	return board, offset == len(data)
}
