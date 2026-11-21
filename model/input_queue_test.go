package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func input(sequence uint32, forward bool) Controls {
	return Controls{Sequence: sequence, Forward: forward}
}

func TestInputQueueHandsOutInputsInOrder(t *testing.T) {
	q := NewInputQueue()
	q.Push(input(2, true))
	q.Push(input(1, false))
	q.Push(input(3, true))

	c, starved := q.Next()
	assert.Equal(t, uint32(1), c.Sequence)
	assert.False(t, starved)
	c, _ = q.Next()
	assert.Equal(t, uint32(2), c.Sequence)
	assert.Equal(t, 1, q.Waiting())
	assert.Equal(t, uint32(2), q.Applied())
}

// a tick without a new input keeps the keys of the last one pressed
func TestInputQueueRepeatsTheLastInputWhenItRunsDry(t *testing.T) {
	q := NewInputQueue()
	c, starved := q.Next()
	assert.True(t, starved)
	assert.Equal(t, Controls{}, c, "nothing was ever pressed")

	q.Push(input(1, true))
	q.Next()
	c, starved = q.Next()
	assert.True(t, starved)
	assert.True(t, c.Forward)
	assert.Equal(t, uint32(1), c.Sequence, "the repeated input is not a new one")
	assert.Equal(t, uint32(1), q.Applied())
	assert.Equal(t, 1, q.Starved)
}

// every key of the repeated input stays pressed, the trigger too: the
// weapon decides when it fires
func TestInputQueueRepeatsEveryKey(t *testing.T) {
	q := NewInputQueue()
	q.Push(Controls{Sequence: 1, Shoot: true, Forward: true})
	c, _ := q.Next()
	assert.True(t, c.Shoot)
	c, _ = q.Next()
	assert.True(t, c.Forward)
	assert.True(t, c.Shoot)
}

// inputs are sent several times so that a lost packet loses nothing
func TestInputQueueIgnoresWhatItAlreadyHas(t *testing.T) {
	q := NewInputQueue()
	q.Push(input(1, true))
	q.Push(input(2, true))
	q.Push(input(1, true))
	q.Push(input(2, true))
	assert.Equal(t, 2, q.Waiting())
	q.Next()
	q.Next()
	// stragglers of inputs that were applied long ago
	q.Push(input(1, false))
	q.Push(input(2, false))
	assert.Equal(t, 0, q.Waiting())
}

func TestInputQueueSkipsAnInputThatNeverArrived(t *testing.T) {
	q := NewInputQueue()
	q.Push(input(5, true))
	q.Push(input(7, true))
	c, _ := q.Next()
	assert.Equal(t, uint32(5), c.Sequence)
	c, starved := q.Next()
	assert.Equal(t, uint32(7), c.Sequence)
	assert.False(t, starved)
}

// a client that sends faster than the server ticks gains nothing
func TestInputQueueKeepsOnlyTheNewestWhenItOverflows(t *testing.T) {
	q := NewInputQueue()
	for i := uint32(1); i <= 1000; i++ {
		q.Push(input(i, true))
	}
	assert.Equal(t, InputQueueCapacity, q.Waiting())
	c, _ := q.Next()
	assert.Equal(t, uint32(1000-InputQueueCapacity+1), c.Sequence)
}

func TestInputQueueIgnoresInputsFromFarInTheFuture(t *testing.T) {
	q := NewInputQueue()
	q.Push(input(1, true))
	q.Next()
	q.Push(input(4000000000, true))
	assert.Equal(t, 0, q.Waiting(), "one bogus number must not lock the player out")
	q.Push(input(2, true))
	assert.Equal(t, 1, q.Waiting())
}

func TestInputQueueStartsOverAfterReset(t *testing.T) {
	q := NewInputQueue()
	q.Push(Controls{Sequence: 9, Forward: true})
	q.Next()
	q.Reset()
	c, starved := q.Next()
	assert.True(t, starved)
	assert.False(t, c.Forward, "a tank does not drive on by itself in the next round")
	// the client keeps counting
	q.Push(input(10, true))
	assert.Equal(t, 1, q.Waiting())
}

// A client that fell silent with a key pressed must not drive on for ever:
// a short gap is bridged, after that the keys count as released.
func TestInputQueueReleasesTheKeysOfASilentClient(t *testing.T) {
	q := NewInputQueue()
	q.Push(Controls{Sequence: 5, Forward: true, Right: true, Shoot: true})
	q.Next()
	for i := 0; i < MaxInputRepeats; i++ {
		c, starved := q.Next()
		assert.True(t, starved)
		assert.True(t, c.Forward, "repeat %d", i+1)
	}
	c, starved := q.Next()
	assert.True(t, starved)
	assert.Equal(t, Controls{Sequence: 5}, c, "no key is pressed, the number of the last input stays")
	assert.Equal(t, uint32(5), q.Applied())

	// it speaks again
	q.Push(Controls{Sequence: 6, Forward: true})
	c, starved = q.Next()
	assert.False(t, starved)
	assert.True(t, c.Forward)
	c, _ = q.Next()
	assert.True(t, c.Forward, "and gaps are bridged again")
}
