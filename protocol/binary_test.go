package protocol

import (
	"encoding/binary"
	"math"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/awdng/triebwerk/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The layouts in this file are mirrored in the client, src/game/net/protocol.js

func TestLanes(t *testing.T) {
	for _, reliable := range []byte{Hello, Welcome, Reject, RoundStart, RoundEnd, Scoreboard, Kill, Denied, Accounts} {
		assert.True(t, IsReliable(reliable), "type %d", reliable)
	}
	for _, unreliable := range []byte{Input, Snapshot, Ping, Pong} {
		assert.False(t, IsReliable(unreliable), "type %d", unreliable)
	}
}

// ---- what clients send ------------------------------------------------------

// the token is everything after the version: the master server's tokens
// are longer than a length byte could say
func TestDecodeHello(t *testing.T) {
	p := NewBinaryProtocol()
	message := p.Decode([]byte{Hello, 2, 'J', 0xc3, 0xa4, 'g', 'e'})
	assert.Equal(t, uint8(Hello), message.MessageType)
	assert.Equal(t, model.Hello{Version: 2, Token: "Jäge"}, message.Body)

	long := strings.Repeat("t", 900)
	assert.Equal(t, model.Hello{Version: 2, Token: long}, p.Decode(append([]byte{Hello, 2}, long...)).Body)
	// no token at all, and a version this server does not speak: the game decides
	assert.Equal(t, model.Hello{Version: 7, Token: ""}, p.Decode([]byte{Hello, 7}).Body)
	// what is not text is not passed on
	assert.Equal(t, model.Hello{Version: 2, Token: "A"}, p.Decode([]byte{Hello, 2, 0xff, 0xfe, 'A'}).Body)
}

func TestDecodeInput(t *testing.T) {
	p := NewBinaryProtocol()
	// input 300 and the two before it: forward+right+shoot, forward, nothing
	message := p.Decode([]byte{Input, 0x2c, 0x01, 0, 0, 3, 1 | 8 | 64, 1, 0})
	assert.Equal(t, uint8(Input), message.MessageType)
	inputs := message.Body.([]model.Controls)
	assert.Equal(t, []model.Controls{
		{Sequence: 300, Forward: true, Right: true, Shoot: true},
		{Sequence: 299, Forward: true},
		{Sequence: 298},
	}, inputs)

	all := p.Decode([]byte{Input, 1, 0, 0, 0, 1, 127}).Body.([]model.Controls)
	assert.Equal(t, []model.Controls{{Sequence: 1, Forward: true, Backward: true, Left: true, Right: true, TurretLeft: true, TurretRight: true, Shoot: true}}, all)
}

func TestDecodeInputNeverCountsBelowOne(t *testing.T) {
	p := NewBinaryProtocol()
	// input 2 with two older ones: there is no input 0
	inputs := p.Decode([]byte{Input, 2, 0, 0, 0, 3, 1, 1, 1}).Body.([]model.Controls)
	assert.Len(t, inputs, 2)
	assert.Equal(t, uint32(1), inputs[1].Sequence)
}

func TestDecodePing(t *testing.T) {
	p := NewBinaryProtocol()
	message := p.Decode([]byte{Ping, 0x78, 0x56, 0x34, 0x12})
	assert.Equal(t, uint8(Ping), message.MessageType)
	assert.Equal(t, model.Ping{ClientTime: 0x12345678}, message.Body)
}

// malformed client messages must never crash the server
func TestDecodeMalformedMessagesDoNotPanic(t *testing.T) {
	p := NewBinaryProtocol()
	inputs := [][]byte{
		{},
		{Hello},
		{Input},
		{Input, 1, 0, 0},
		{Input, 1, 0, 0, 0},
		{Input, 1, 0, 0, 0, 0},
		{Input, 1, 0, 0, 0, 3, 1},
		{Input, 1, 0, 0, 0, 200, 1, 1, 1},
		{Ping},
		{Ping, 1, 2},
		{0, 0, 'o', 'l', 'd'},
		{Snapshot, 1, 2, 3},
		{99, 1, 2, 3},
	}
	for _, data := range inputs {
		assert.NotPanics(t, func() { p.Decode(data) }, "input %v", data)
	}
	for _, data := range [][]byte{{}, {Input, 1, 0, 0}, {Input, 1, 0, 0, 0, 0}, {Ping, 1}, {99, 1, 2, 3}, {0, 0, 'o', 'l', 'd'}} {
		message := p.Decode(data)
		assert.Equal(t, uint8(Malformed), message.MessageType, "input %v", data)
		assert.Nil(t, message.Body, "input %v", data)
	}
	// more inputs announced than sent or allowed: the ones that are there count
	assert.Len(t, p.Decode([]byte{Input, 9, 0, 0, 0, 3, 1}).Body, 1)
	assert.Len(t, p.Decode([]byte{Input, 9, 0, 0, 0, 200, 1, 1, 1, 1, 1, 1}).Body, MaxInputsPerMessage)
}

// ---- what the server sends --------------------------------------------------

func TestEncodeWelcome(t *testing.T) {
	p := NewBinaryProtocol()
	data := p.Welcome(7, 1000, 300*time.Second)
	assert.Equal(t, []byte{Welcome, Version, 7, model.TickMillis}, data[:4])
	assert.Equal(t, uint32(300000), binary.LittleEndian.Uint32(data[4:8]))
	assert.Equal(t, uint32(1000), binary.LittleEndian.Uint32(data[8:12]))
	assert.Len(t, data, 12)
}

func TestEncodeSmallMessages(t *testing.T) {
	p := NewBinaryProtocol()
	assert.Equal(t, []byte{Reject, Version}, p.Reject())
	assert.Equal(t, []byte{RoundStart, 0xe8, 0x03, 0, 0}, p.RoundStart(1000))
	assert.Equal(t, []byte{RoundEnd, 0xe9, 0x03, 0, 0}, p.RoundEnd(1001))
	assert.Equal(t, []byte{Kill, 4, 9}, p.Kill(model.Kill{Killer: 4, Victim: 9}))
	assert.Equal(t, []byte{Pong, 0x78, 0x56, 0x34, 0x12}, p.Pong(0x12345678))
}

func player(id int, x, y, rotation, turret float32, health int, shots uint8) *model.Player {
	p := model.NewPlayer(id, x, y, nil)
	p.Collider.Rotation = rotation
	p.Collider.TurretRotation = turret
	p.Health = health
	p.Shots = shots
	return p
}

func TestEncodeSnapshot(t *testing.T) {
	p := NewBinaryProtocol()
	body := p.SnapshotBody([]*model.Player{
		player(3, 1.5, -2.25, 0.5, 1.25, 75, 4),
		player(12, 100, 200, -3, 3, 0, 255),
	})
	data := p.Snapshot(70000, 300, 2, body)

	assert.Equal(t, byte(Snapshot), data[0])
	assert.Equal(t, uint32(70000), binary.LittleEndian.Uint32(data[1:5]), "tick")
	assert.Equal(t, uint32(300), binary.LittleEndian.Uint32(data[5:9]), "last input applied for the receiver")
	assert.Equal(t, byte(2), data[9], "inputs of the receiver that wait")
	assert.Equal(t, byte(2), data[10], "tanks")
	assert.Len(t, data, 11+2*SnapshotEntrySize)

	float := func(offset int) float32 { return math.Float32frombits(binary.LittleEndian.Uint32(data[offset:])) }
	first := 11
	assert.Equal(t, byte(3), data[first])
	assert.Equal(t, float32(1.5), float(first+1))
	assert.Equal(t, float32(-2.25), float(first+5))
	assert.Equal(t, float32(0.5), float(first+9))
	assert.Equal(t, float32(1.25), float(first+13))
	assert.Equal(t, byte(75), data[first+17])
	assert.Equal(t, byte(4), data[first+18])
	second := first + SnapshotEntrySize
	assert.Equal(t, byte(12), data[second])
	assert.Equal(t, byte(0), data[second+17])
	assert.Equal(t, byte(255), data[second+18])

	// the same body goes to every client, each with its own header
	other := p.Snapshot(70000, 9, WaitingStarved, body)
	assert.Equal(t, data[10:], other[10:])
	assert.Equal(t, byte(255), other[9])
}

func TestDecodeSnapshotReadsWhatWasEncoded(t *testing.T) {
	p := NewBinaryProtocol()
	body := p.SnapshotBody([]*model.Player{player(3, 1.5, -2.25, 0.5, 1.25, 75, 4), player(12, 100, 200, -3, 3, 0, 255)})
	snapshot, ok := DecodeSnapshot(p.Snapshot(70000, 300, 2, body))
	assert.True(t, ok)
	assert.Equal(t, SnapshotMessage{Tick: 70000, Ack: 300, Waiting: 2, Players: []SnapshotEntry{
		{ID: 3, X: 1.5, Y: -2.25, Rotation: 0.5, TurretRotation: 1.25, Health: 75, Shots: 4},
		{ID: 12, X: 100, Y: 200, Rotation: -3, TurretRotation: 3, Health: 0, Shots: 255},
	}}, snapshot)

	data := p.Snapshot(1, 1, 0, body)
	_, ok = DecodeSnapshot(data[:len(data)-1])
	assert.False(t, ok, "cut off")
	_, ok = DecodeSnapshot(append(append([]byte{}, data...), 0))
	assert.False(t, ok, "bytes left over")
	_, ok = DecodeSnapshot([]byte{Snapshot, 1})
	assert.False(t, ok)
	empty, ok := DecodeSnapshot(p.Snapshot(5, 0, 0, p.SnapshotBody(nil)))
	assert.True(t, ok)
	assert.Empty(t, empty.Players)
}

func TestSnapshotStaysInOneDatagram(t *testing.T) {
	p := NewBinaryProtocol()
	players := make([]*model.Player, 300)
	for i := range players {
		players[i] = player(i, 0, 0, 0, 0, 100, 0)
	}
	data := p.Snapshot(1, 1, 0, p.SnapshotBody(players))
	// a QUIC packet carries about 1200 bytes, some of them are its own
	assert.LessOrEqual(t, len(data), 1100)
	snapshot, ok := DecodeSnapshot(data)
	assert.True(t, ok)
	assert.Equal(t, MaxSnapshotPlayers, len(snapshot.Players))

	twelve := p.Snapshot(1, 1, 0, p.SnapshotBody(players[:12]))
	assert.Equal(t, 11+12*19, len(twelve))
}

func scoreboardMessage(players ...model.ScoreboardEntry) *model.Scoreboard {
	return &model.Scoreboard{RoundLength: 300 * time.Second, Players: players}
}

func TestEncodeScoreboard(t *testing.T) {
	p := NewBinaryProtocol()
	data := p.Scoreboard(scoreboardMessage(
		model.ScoreboardEntry{ID: 3, Kills: 7, Deaths: 2, Name: "kayoone"},
		model.ScoreboardEntry{ID: 12, Kills: 300, Deaths: 0, Name: "Jäger"},
	))
	assert.Equal(t, byte(Scoreboard), data[0])
	assert.Equal(t, uint32(300000), binary.LittleEndian.Uint32(data[1:5]))
	assert.Equal(t, byte(2), data[5])
	// id, kills, deaths, length of the name, name
	assert.Equal(t, []byte{3, 7, 0, 2, 0, 7}, data[6:12])
	assert.Equal(t, "kayoone", string(data[12:19]))
	assert.Equal(t, []byte{12, 0x2c, 0x01, 0, 0, 6}, data[19:25])
	assert.Equal(t, "Jäger", string(data[25:31]))
	assert.Len(t, data, 31)
}

func TestEncodeScoreboardWithoutPlayers(t *testing.T) {
	p := NewBinaryProtocol()
	assert.Equal(t, []byte{Scoreboard, 0xe0, 0x93, 0x04, 0, 0}, p.Scoreboard(scoreboardMessage()))
}

// names are what players typed: too long, cut in the middle of a character, not text at all
func TestEncodeScoreboardCutsNamesToValidText(t *testing.T) {
	p := NewBinaryProtocol()
	names := map[string]string{
		"":                            "",
		"short":                       "short",
		strings.Repeat("x", 120):      strings.Repeat("x", 88),
		strings.Repeat("x", 87) + "ä": strings.Repeat("x", 87),
		strings.Repeat("ä", 50):       strings.Repeat("ä", 44),
		"\xff\xfeA":                   "A",
		strings.Repeat("🎯", 23):       strings.Repeat("🎯", 22),
	}
	for name, want := range names {
		data := p.Scoreboard(scoreboardMessage(model.ScoreboardEntry{ID: 1, Name: name}))
		length := int(data[11])
		got := string(data[12:])
		assert.Equal(t, want, got, "name %q", name)
		assert.Equal(t, len(want), length, "name %q", name)
		assert.LessOrEqual(t, length, 88)
		assert.True(t, utf8.ValidString(got), "name %q", name)
	}
}

func TestEncodeScoreboardStaysInItsFields(t *testing.T) {
	p := NewBinaryProtocol()
	players := make([]model.ScoreboardEntry, 300)
	for i := range players {
		players[i] = model.ScoreboardEntry{ID: i, Kills: 70000, Deaths: -1}
	}
	data := p.Scoreboard(scoreboardMessage(players...))
	// the count is one byte
	assert.Equal(t, byte(255), data[5])
	assert.Len(t, data, 6+255*6)
	// kills and deaths are two bytes each and never wrap
	assert.Equal(t, []byte{0xff, 0xff, 0, 0}, data[7:11])
}

func TestDecodeScoreboardReadsWhatWasEncoded(t *testing.T) {
	p := NewBinaryProtocol()
	data := p.Scoreboard(scoreboardMessage(
		model.ScoreboardEntry{ID: 3, Kills: 7, Deaths: 2, Name: "kayoone"},
		model.ScoreboardEntry{ID: 12, Kills: 300, Deaths: 0, Name: "Jäger"},
	))
	board, ok := DecodeScoreboard(data)
	assert.True(t, ok)
	assert.Equal(t, 300*time.Second, board.RoundLength)
	assert.Equal(t, []model.ScoreboardEntry{{ID: 3, Kills: 7, Deaths: 2, Name: "kayoone"}, {ID: 12, Kills: 300, Deaths: 0, Name: "Jäger"}}, board.Players)

	// strict: every byte is accounted for and names are text
	_, ok = DecodeScoreboard(data[:len(data)-1])
	assert.False(t, ok, "cut off")
	_, ok = DecodeScoreboard(append(append([]byte{}, data...), 0))
	assert.False(t, ok, "bytes left over")
	broken := append([]byte{}, data...)
	broken[12] = 0xff
	_, ok = DecodeScoreboard(broken)
	assert.False(t, ok, "a name that is not utf8")
	_, ok = DecodeScoreboard([]byte{Scoreboard, 1, 2})
	assert.False(t, ok)
}

// ---- what a client of this package sends (bots, tests) -----------------------

func TestClientMessagesAreUnderstoodByTheServer(t *testing.T) {
	p := NewBinaryProtocol()
	assert.Equal(t, model.Hello{Version: Version, Token: "load-01"}, p.Decode(EncodeHello("load-01")).Body)
	assert.Equal(t, model.Ping{ClientTime: 77}, p.Decode(EncodePing(77)).Body)

	inputs := p.Decode(EncodeInput(10, []byte{BitForward | BitShoot, BitBackward, 0})).Body.([]model.Controls)
	assert.Equal(t, []model.Controls{
		{Sequence: 10, Forward: true, Shoot: true},
		{Sequence: 9, Backward: true},
		{Sequence: 8},
	}, inputs)
}

// the same bytes as in the client's protocol.test.js
func TestDeniedSaysWhyAPlayerIsTurnedAway(t *testing.T) {
	p := NewBinaryProtocol()
	assert.Equal(t, []byte{Denied, DeniedNotAuthorized}, p.Denied(DeniedNotAuthorized))
	assert.Equal(t, []byte{8, 1}, p.Denied(DeniedNotAuthorized))
	assert.Equal(t, []byte{8, 2}, p.Denied(DeniedNoMaster))
	// an account plays one match at a time
	assert.Equal(t, []byte{8, 3}, p.Denied(DeniedElsewhere))
	assert.Equal(t, []byte{8, 4}, p.Denied(DeniedReplaced))
}

// The master server gives a guest who uses the name of an account that name
// with " (guest)" behind it. The longest name there is has 20 letters of
// four bytes each; it reaches every client whole, or a guest could not be
// told from the account.
func TestScoreboardCarriesTheLongestNameTheMasterGivesWhole(t *testing.T) {
	p := NewBinaryProtocol()
	account := strings.Repeat("𝔸", 20)
	guest := "Александр Иванович (guest)"
	data := p.Scoreboard(scoreboardMessage(
		model.ScoreboardEntry{ID: 1, Name: account + " (guest)"},
		model.ScoreboardEntry{ID: 2, Name: guest},
	))
	board, ok := DecodeScoreboard(data)
	require.True(t, ok)
	assert.Equal(t, account+" (guest)", board.Players[0].Name)
	assert.Equal(t, 88, len(board.Players[0].Name))
	assert.Equal(t, guest, board.Players[1].Name)
}

// Who has an account is told beside the scoreboard, in a message of its
// own: a client or bot from before reads the scoreboard as it always did
// and ignores the type it does not know.
func TestAccountsListsWhoOnTheScoreboardPlaysWithAnAccount(t *testing.T) {
	p := NewBinaryProtocol()
	board := scoreboardMessage(
		model.ScoreboardEntry{ID: 3, Kills: 7, Deaths: 2, Name: "kayoone", Account: true},
		model.ScoreboardEntry{ID: 7, Name: "load-01"},
		model.ScoreboardEntry{ID: 12, Kills: 300, Name: "Jäger", Account: true},
	)
	data := p.Accounts(board)
	// count, then the ids: everybody else is a guest
	assert.Equal(t, []byte{9, 2, 3, 12}, data)
	assert.Equal(t, byte(Accounts), data[0])
	ids, ok := DecodeAccounts(data)
	require.True(t, ok)
	assert.Equal(t, []int{3, 12}, ids)

	assert.Equal(t, []byte{Accounts, 0}, p.Accounts(scoreboardMessage(model.ScoreboardEntry{ID: 7, Name: "load-01"})))
	assert.Equal(t, []byte{Accounts, 0}, p.Accounts(scoreboardMessage()))
	ids, ok = DecodeAccounts([]byte{Accounts, 0})
	require.True(t, ok)
	assert.Empty(t, ids)

	// the scoreboard itself has not changed
	guests := scoreboardMessage(
		model.ScoreboardEntry{ID: 3, Kills: 7, Deaths: 2, Name: "kayoone"},
		model.ScoreboardEntry{ID: 7, Name: "load-01"},
		model.ScoreboardEntry{ID: 12, Kills: 300, Name: "Jäger"},
	)
	assert.Equal(t, p.Scoreboard(guests), p.Scoreboard(board))
}

func TestDecodeAccountsIsStrict(t *testing.T) {
	for _, bad := range [][]byte{{}, {Accounts}, {Accounts, 2, 3}, {Accounts, 1, 3, 4}, {Scoreboard, 0}, {Accounts, 0, 0}} {
		_, ok := DecodeAccounts(bad)
		assert.False(t, ok, "%v", bad)
	}
}

func TestAccountsOfMoreThan255PlayersStayInTheirField(t *testing.T) {
	players := make([]model.ScoreboardEntry, 300)
	for i := range players {
		players[i] = model.ScoreboardEntry{ID: i % 250, Account: true}
	}
	data := NewBinaryProtocol().Accounts(scoreboardMessage(players...))
	assert.Len(t, data, 2+255)
	_, ok := DecodeAccounts(data)
	assert.True(t, ok)
}
