package model

// InputQueueCapacity is how many inputs of a player wait for their tick. The
// server applies one per tick; more than a few waiting only add delay.
const InputQueueCapacity = 4

// an input numbered this far beyond the last one is not from this session
const maxInputLead = 1000

// InputQueue holds the inputs of one player between their arrival and the
// tick that applies them. Inputs are numbered by the client and may arrive
// late, twice or not at all; the queue hands them out in order, one per tick.
type InputQueue struct {
	// waiting inputs, lowest sequence first
	pending []Controls
	// the input of the last tick, repeated while nothing new is waiting
	last Controls
	// Starved counts the ticks that had to repeat an input
	Starved int
}

// NewInputQueue returns an empty queue
func NewInputQueue() *InputQueue {
	return &InputQueue{}
}

// Push adds an input unless it was applied already, is waiting already or
// carries a number that can not be right.
func (q *InputQueue) Push(c Controls) {
	applied := q.last.Sequence
	if c.Sequence <= applied {
		return
	}
	if applied != 0 && c.Sequence-applied > maxInputLead {
		return
	}
	at := len(q.pending)
	for i, waiting := range q.pending {
		if waiting.Sequence == c.Sequence {
			return
		}
		if waiting.Sequence > c.Sequence {
			at = i
			break
		}
	}
	q.pending = append(q.pending, Controls{})
	copy(q.pending[at+1:], q.pending[at:])
	q.pending[at] = c
	// a client that sends faster than the server ticks only loses its oldest inputs
	if len(q.pending) > InputQueueCapacity {
		q.pending = q.pending[len(q.pending)-InputQueueCapacity:]
	}
}

// Next returns the input for the coming tick. With nothing waiting the last
// input is returned again: the keys stay as they were, and starved is true.
func (q *InputQueue) Next() (c Controls, starved bool) {
	if len(q.pending) == 0 {
		if q.last.Sequence != 0 {
			q.Starved++
		}
		return q.last, true
	}
	q.last = q.pending[0]
	q.pending = q.pending[1:]
	return q.last, false
}

// Waiting is the number of inputs that have not been applied yet
func (q *InputQueue) Waiting() int {
	return len(q.pending)
}

// Applied is the number of the last input that was handed out
func (q *InputQueue) Applied() uint32 {
	return q.last.Sequence
}

// Reset forgets everything: no key is pressed, any number may come next
func (q *InputQueue) Reset() {
	q.pending = nil
	q.last = Controls{}
}
