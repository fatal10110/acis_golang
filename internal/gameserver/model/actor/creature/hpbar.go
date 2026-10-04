package creature

import "sync"

// hpBarSize is the number of pixels in the client's target health bar.
const hpBarSize = 352.0

// HPBar tracks which segment of the client's target health bar an actor's
// current HP was last reported in, so the players watching that bar are only
// sent HP changes that move it.
//
// The zero value is an uncalibrated bar: its segment width is zero, so it
// reports nearly every change. Only an actor that calls Calibrate gets the
// one-segment-per-pixel filtering.
type HPBar struct {
	mu                           sync.Mutex
	interval, incCheck, decCheck float64
}

// Calibrate sizes b's segments for maxHP and marks full HP as last reported.
func (b *HPBar) Calibrate(maxHP float64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.interval = maxHP / hpBarSize
	b.incCheck = maxHP
	b.decCheck = maxHP - b.interval
}

// Report reads current HP through current and reports it, int-truncated,
// with whether the players watching the bar must be sent it, moving the
// tracked segment to its segment when they must. The read and the check run
// under one lock, so concurrent reports settle in the order HP was read.
// Near-dead HP and a bar shorter than one pixel per point always report.
// maxHP is the actor's integer maximum HP.
func (b *HPBar) Report(current func() float64, maxHP float64) (int, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	hp := current()
	return int(hp), b.need(hp, maxHP)
}

// Publish is Report for delivery: when the players watching the bar must be
// sent the HP current reads, it hands the int-truncated value to send while
// still holding the bar's lock. Reading, gating and enqueueing are then one
// step, so concurrent publishes for one creature reach every watcher in the
// order HP was read: the last value a watcher gets is never older than one
// it already got. send must not block, and must not take a lock held while
// Publish is called.
func (b *HPBar) Publish(current func() float64, maxHP float64, send func(hp int)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	hp := current()
	if b.need(hp, maxHP) {
		send(int(hp))
	}
}

func (b *HPBar) need(hp, maxHP float64) bool {
	if hp <= 1.0 || maxHP < hpBarSize {
		return true
	}
	if hp > b.decCheck && hp < b.incCheck {
		return false
	}
	if hp == maxHP {
		b.incCheck = hp + 1
		b.decCheck = hp - b.interval
	} else {
		// An uncalibrated bar has a zero interval, so both checks fall back
		// to zero whatever the segment index works out to.
		b.decCheck = b.interval * float64(int(hp/b.interval))
		b.incCheck = b.decCheck + b.interval
	}
	return true
}
