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
	assert.Equal(t, uint32(2), q.Applied(), "but it stands in for the next one")
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
	assert.Equal(t, uint32(5+MaxInputRepeats), q.Applied(), "the repeats stood in for ten inputs, no more")

	// it speaks again, with the number it would have had anyway
	q.Push(Controls{Sequence: 6, Forward: true})
	c, starved = q.Next()
	assert.False(t, starved)
	assert.True(t, c.Forward)
	assert.Equal(t, uint32(6), q.Applied())
	c, _ = q.Next()
	assert.True(t, c.Forward, "and gaps are bridged again")
}

// A tick without an input is bridged with the keys of the last one. When the
// late input arrives with the next one, both are kept: one for the coming
// tick, one in reserve, so that the next late packet finds the queue ready.
func TestInputQueueBuildsItsReserveFromALateInput(t *testing.T) {
	q := NewInputQueue()
	q.Push(input(1, true))
	q.Next()
	// input 2 is late
	c, starved := q.Next()
	assert.True(t, starved)
	assert.True(t, c.Forward)
	assert.Equal(t, uint32(2), q.Applied(), "the bridged tick counts as input 2 for now")

	// it arrives together with input 3
	q.Push(input(2, false))
	q.Push(input(3, true))
	c, starved = q.Next()
	assert.False(t, starved)
	assert.Equal(t, uint32(2), c.Sequence)
	assert.False(t, c.Forward)
	assert.Equal(t, 1, q.Waiting(), "one input in reserve")
	assert.Equal(t, uint32(2), q.Applied())
}

// A frame that took 300 ms: nine ticks bridged, then nine inputs at once.
// The server has simulated the ticks they were meant for. Kept, they would
// only wait: every later input a quarter of a second behind them.
func TestInputQueueDropsLateInputsThatWouldOnlyWait(t *testing.T) {
	q := NewInputQueue()
	q.Push(input(1, true))
	q.Next()
	for i := 0; i < 9; i++ {
		q.Next()
	}
	assert.Equal(t, uint32(10), q.Applied())
	for sequence := uint32(2); sequence <= 10; sequence++ {
		q.Push(input(sequence, true))
	}
	c, starved := q.Next()
	assert.False(t, starved)
	assert.Equal(t, uint32(9), c.Sequence, "the two newest stay: one for this tick")
	assert.Equal(t, 1, q.Waiting(), "and one in reserve")
	c, _ = q.Next()
	assert.Equal(t, uint32(10), c.Sequence)
}

// only inputs whose ticks were bridged are dropped, not ones that are merely early
func TestInputQueueKeepsInputsThatAreNotLate(t *testing.T) {
	q := NewInputQueue()
	q.Push(input(1, true))
	q.Next()
	q.Next() // one tick bridged
	for sequence := uint32(2); sequence <= 6; sequence++ {
		q.Push(input(sequence, true))
	}
	c, _ := q.Next()
	assert.Equal(t, uint32(3), c.Sequence, "input 2 was late, the others are for ticks to come")
	assert.Equal(t, 3, q.Waiting())
}

// the client is followed again when it ran fewer ticks than the server bridged
func TestInputQueueFollowsAClientThatFellBehind(t *testing.T) {
	q := NewInputQueue()
	q.Push(input(1, true))
	q.Next()
	for i := 0; i < 5; i++ {
		q.Next()
	}
	assert.Equal(t, uint32(6), q.Applied())
	// the client ran only two ticks in that time
	q.Push(input(2, false))
	q.Push(input(3, false))
	c, starved := q.Next()
	assert.False(t, starved)
	assert.Equal(t, uint32(2), c.Sequence)
	assert.False(t, c.Forward)
	assert.Equal(t, uint32(2), q.Applied(), "the numbering follows the client again")
	c, _ = q.Next()
	assert.Equal(t, uint32(3), c.Sequence)
}

// an input that is merely late, with none after it, is applied: better late than a guess
func TestInputQueueAppliesALateInputWhenItIsAllThereIs(t *testing.T) {
	q := NewInputQueue()
	q.Push(input(1, true))
	q.Next()
	q.Next() // bridged
	q.Push(input(2, false))
	c, starved := q.Next()
	assert.False(t, starved)
	assert.Equal(t, uint32(2), c.Sequence)
	assert.False(t, c.Forward)
}

// bridging has an end: a client that is gone is not waited for
func TestInputQueueBridgesOnlyAsLongAsItRepeats(t *testing.T) {
	q := NewInputQueue()
	q.Push(input(1, true))
	q.Next()
	for i := 0; i < MaxInputRepeats+50; i++ {
		q.Next()
	}
	assert.Equal(t, uint32(1+MaxInputRepeats), q.Applied())
	// a minute later the client is back, with the numbers it had
	q.Push(input(2, true))
	q.Push(input(3, true))
	q.Push(input(4, true))
	c, starved := q.Next()
	assert.False(t, starved)
	assert.Equal(t, uint32(3), c.Sequence)
	assert.Equal(t, 1, q.Waiting())
}

func TestInputQueueBridgesNothingBeforeTheFirstInput(t *testing.T) {
	q := NewInputQueue()
	for i := 0; i < 5; i++ {
		q.Next()
	}
	assert.Equal(t, uint32(0), q.Applied())
	q.Push(input(1, true))
	q.Push(input(2, true))
	c, _ := q.Next()
	assert.Equal(t, uint32(1), c.Sequence)
	assert.Equal(t, 1, q.Waiting())
}

// The inputs of a long frame do not all arrive in the same tick. The first
// ones must not make the queue forget which ticks it bridged.
func TestInputQueueRemembersBridgedTicksWhenTheBurstArrivesInParts(t *testing.T) {
	q := NewInputQueue()
	q.Push(input(1, true))
	q.Next()
	for i := 0; i < 5; i++ {
		q.Next() // inputs 2 to 6 are late
	}
	q.Push(input(2, true))
	q.Push(input(3, true))
	c, _ := q.Next()
	assert.Equal(t, uint32(2), c.Sequence)

	// the rest of the burst, one tick later
	for sequence := uint32(4); sequence <= 10; sequence++ {
		q.Push(input(sequence, true))
	}
	c, _ = q.Next()
	assert.Equal(t, uint32(7), c.Sequence, "3 to 6 were for ticks that are done")
	assert.Equal(t, 3, q.Waiting(), "8, 9 and 10 are for ticks to come")
}
