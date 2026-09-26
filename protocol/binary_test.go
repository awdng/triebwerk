package protocol

import (
	"testing"

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
