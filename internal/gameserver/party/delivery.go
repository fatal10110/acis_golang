package party

import "sync"

// Delivery hands the notices of party changes to the clients in the order
// the changes were decided, whichever goroutines decide them, so that no
// member sees a later change before an earlier one (#5144): a join decided
// before a leave lists the leaver in the joiner's window before the leaver's
// row is deleted from it. The zero Delivery is ready to use.
type Delivery struct {
	mu       sync.Mutex
	pending  [][]Notice
	draining bool
}

// Change runs decide, which makes party changes and returns their notices,
// then delivers them through apply once every notice decided before them is
// delivered. Decisions are ordered by mu, which decide runs under, so decide
// must not call Change itself; apply may, and the notices it decides then
// follow the ones being applied.
//
// One goroutine at a time applies notices, its own and those others
// decided meanwhile, in decision order, and every caller passes the same
// apply. A caller that finds another goroutine applying leaves its notices
// to that goroutine and returns without waiting for them, so no caller ever
// blocks on another's delivery.
func (d *Delivery) Change(decide func() []Notice, apply func([]Notice)) {
	d.mu.Lock()
	if n := decide(); len(n) > 0 {
		d.pending = append(d.pending, n)
	}
	if d.draining {
		d.mu.Unlock()
		return
	}
	d.draining = true
	d.mu.Unlock()

	drained := false
	defer func() {
		// A panicking apply leaves what is still pending to the next
		// Change instead of to no one.
		if !drained {
			d.mu.Lock()
			d.draining = false
			d.mu.Unlock()
		}
	}()
	for {
		d.mu.Lock()
		if len(d.pending) == 0 {
			d.draining = false
			d.mu.Unlock()
			drained = true
			return
		}
		next := d.pending[0]
		d.pending[0] = nil
		d.pending = d.pending[1:]
		d.mu.Unlock()
		apply(next)
	}
}
