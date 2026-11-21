package model

// InputQueueCapacity is how many inputs of a player wait for their tick. The
// server applies one per tick. A client sends the inputs of all ticks of a
// frame at once, so at 10 frames per second three arrive together and one
// should still be in reserve. Clients keep the queue short themselves (the
// snapshots tell them how full it is); what does not fit is the oldest.
const InputQueueCapacity = 8

// an input numbered this far beyond the last one is not from this session
const maxInputLead = 1000

// MaxInputRepeats is how many ticks in a row the last input is applied again
// when none arrives, a third of a second. That bridges a late packet. After
// it the keys count as released: a client that is gone does not drive on.
const MaxInputRepeats = 10

// InputQueue holds the inputs of one player between their arrival and the
// tick that applies them. Inputs are numbered by the client and may arrive
// late, twice or not at all; the queue hands them out in order, one per tick.
//
// A tick for which no input has arrived is bridged: the keys of the last
// input are applied again, and the tick counts as that input (see Applied).
// When the late input arrives after all, there are two cases. If it would
// leave the queue no longer than one input in reserve, it is kept: the
// reserve is what makes the next late packet harmless. If it would only
// wait, it is dropped. That is the burst after a frame that took 300 ms:
// nine inputs for ticks the server has simulated already. Kept, every later
// input would wait a quarter of a second behind them.
type InputQueue struct {
	// waiting inputs, lowest sequence first
	pending []Controls
	// the last input a tick took from the queue, repeated while nothing new is waiting
	last Controls
	// ticks in a row that were bridged with it, at most MaxInputRepeats
	repeats int
	// the highest input number a bridged tick stood in for. It outlives the
	// first late input that is applied: the inputs of a long frame arrive
	// over more than one tick.
	bridged uint32
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
	// an input older than the last one that was taken; one whose tick was
	// only bridged is kept until Next decides
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

// Next returns the input for the coming tick. With nothing waiting the tick
// is bridged: the last input is returned again, the keys stay as they were,
// and starved is true. After MaxInputRepeats such ticks no key is pressed
// any more and nothing is bridged: the client is gone, not late.
func (q *InputQueue) Next() (c Controls, starved bool) {
	if len(q.pending) == 0 {
		// before the first input there is nothing to repeat and nothing to bridge
		if q.last.Sequence == 0 {
			return q.last, true
		}
		q.Starved++
		if q.repeats >= MaxInputRepeats {
			return Controls{Sequence: q.last.Sequence}, true
		}
		q.repeats++
		q.bridged = q.Applied()
		return q.last, true
	}
	// Inputs for ticks that were bridged are dropped as far as they would
	// only wait. Two stay in any case, one for this tick and one in reserve:
	// a client that is late builds its reserve from them, and one that ran
	// fewer ticks than the server bridged is followed again, not waited for.
	for len(q.pending) > 2 && q.pending[0].Sequence <= q.bridged {
		q.pending = q.pending[1:]
	}
	q.last = q.pending[0]
	q.pending = q.pending[1:]
	q.repeats = 0
	return q.last, false
}

// Waiting is the number of inputs that have not been applied yet
func (q *InputQueue) Waiting() int {
	return len(q.pending)
}

// Applied is the number of the last input that went into the game: the
// last one a tick took from the queue, plus one for every tick that was
// bridged since. Clients compare what they predicted for that input with
// what the server has. The number goes back when a late input is applied
// after its tick was bridged; clients then see that the server made a step
// more than they did.
func (q *InputQueue) Applied() uint32 {
	return q.last.Sequence + uint32(q.repeats)
}

// Reset forgets everything: no key is pressed, any number may come next
func (q *InputQueue) Reset() {
	q.pending = nil
	q.last = Controls{}
	q.repeats = 0
	q.bridged = 0
}
