package protocol

import (
	"encoding/binary"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/awdng/triebwerk/model"
	"github.com/stretchr/testify/assert"
)

// malformed client messages must never crash the server
func TestDecodeMalformedMessagesDoNotPanic(t *testing.T) {
	p := NewBinaryProtocol()
	inputs := [][]byte{
		{},
		{0},
		{0, 1},
		{0, 1, 1, 1, 1, 1, 1, 1, 1},
		{0, 1, 1, 1, 1, 1, 1, 1, 1, 0, 0},
		{0, 5},
		{0, 5, 1, 2},
		{0, 0},
		{0, 99},
	}
	for _, data := range inputs {
		assert.NotPanics(t, func() { p.Decode(data) }, "input %v", data)
	}
	assert.Nil(t, p.Decode([]byte{0, 1, 1}).Body)
	assert.Nil(t, p.Decode([]byte{0, 5}).Body)
	assert.Nil(t, p.Decode([]byte{}).Body)
}

func TestDecodePlayerInput(t *testing.T) {
	p := NewBinaryProtocol()
	message := p.Decode([]byte{0, 1, 1, 0, 0, 1, 0, 1, 1, 0, 0, 1, 2})
	controls := message.Body.(model.Controls)
	assert.True(t, controls.Forward)
	assert.False(t, controls.Backward)
	assert.True(t, controls.Right)
	assert.True(t, controls.TurretLeft)
	assert.True(t, controls.Shoot)
	assert.Equal(t, uint32(258), controls.Sequence)
}

func TestEncodePlayerStateUsesFiredFlag(t *testing.T) {
	p := NewBinaryProtocol()
	player := model.NewPlayer(3, 1, 2, nil)
	player.Control.Shoot = false
	player.Fired = true
	data := p.Encode(player.ID, 1000, &model.NetworkMessage{MessageType: 1, Body: player})
	assert.Len(t, data, 36)
	assert.Equal(t, byte(3), data[0])
	assert.Equal(t, byte(1), data[34], "shooting byte must reflect the fired flag")
	assert.Equal(t, byte(100), data[35])
}

func scoreboardMessage(players ...model.ScoreboardEntry) *model.NetworkMessage {
	return &model.NetworkMessage{MessageType: 8, Body: &model.Scoreboard{RoundLength: 300 * time.Second, Players: players}}
}

// The layout is mirrored in the client, src/game/net/protocol.js
func TestEncodeScoreboard(t *testing.T) {
	p := NewBinaryProtocol()
	data := p.Encode(0, 1234, scoreboardMessage(
		model.ScoreboardEntry{ID: 3, Kills: 7, Deaths: 2, Name: "kayoone"},
		model.ScoreboardEntry{ID: 12, Kills: 300, Deaths: 0, Name: "Jäger"},
	))
	// header: player id 0, type 8, game time little endian
	assert.Equal(t, []byte{0, 8, 0xd2, 0x04, 0, 0}, data[:6])
	assert.Equal(t, uint32(300000), binary.LittleEndian.Uint32(data[6:10]))
	assert.Equal(t, byte(2), data[10])
	// id, kills, deaths, length of the name, name
	assert.Equal(t, []byte{3, 7, 0, 2, 0, 7}, data[11:17])
	assert.Equal(t, "kayoone", string(data[17:24]))
	assert.Equal(t, []byte{12, 0x2c, 0x01, 0, 0, 6}, data[24:30])
	assert.Equal(t, "Jäger", string(data[30:36]))
	assert.Len(t, data, 36)
}

func TestEncodeScoreboardWithoutPlayers(t *testing.T) {
	p := NewBinaryProtocol()
	data := p.Encode(0, 0, scoreboardMessage())
	assert.Equal(t, []byte{0, 8, 0, 0, 0, 0, 0xe0, 0x93, 0x04, 0, 0}, data)
}

// names are what players typed: too long, cut in the middle of a character, not text at all
func TestEncodeScoreboardCutsNamesToValidText(t *testing.T) {
	p := NewBinaryProtocol()
	names := map[string]string{
		"":                            "",
		"short":                       "short",
		strings.Repeat("x", 40):       strings.Repeat("x", 24),
		strings.Repeat("x", 23) + "ä": strings.Repeat("x", 23),
		strings.Repeat("ä", 20):       strings.Repeat("ä", 12),
		"\xff\xfeA":                   "A",
		strings.Repeat("🎯", 7):        strings.Repeat("🎯", 6),
	}
	for name, want := range names {
		data := p.Encode(0, 0, scoreboardMessage(model.ScoreboardEntry{ID: 1, Name: name}))
		length := int(data[16])
		got := string(data[17:])
		assert.Equal(t, want, got, "name %q", name)
		assert.Equal(t, len(want), length, "name %q", name)
		assert.LessOrEqual(t, length, 24)
		assert.True(t, utf8.ValidString(got), "name %q", name)
	}
}

func TestEncodeScoreboardStaysInItsFields(t *testing.T) {
	p := NewBinaryProtocol()
	players := make([]model.ScoreboardEntry, 300)
	for i := range players {
		players[i] = model.ScoreboardEntry{ID: i, Kills: 70000, Deaths: -1}
	}
	data := p.Encode(0, 0, scoreboardMessage(players...))
	// the count is one byte
	assert.Equal(t, byte(255), data[10])
	assert.Len(t, data, 11+255*6)
	// kills and deaths are two bytes each and never wrap
	assert.Equal(t, []byte{0xff, 0xff, 0, 0}, data[12:16])
}

func TestEncodeKill(t *testing.T) {
	p := NewBinaryProtocol()
	data := p.Encode(0, 77, &model.NetworkMessage{MessageType: 9, Body: model.Kill{Killer: 4, Victim: 9}})
	assert.Equal(t, []byte{0, 9, 77, 0, 0, 0, 4, 9}, data)
}
