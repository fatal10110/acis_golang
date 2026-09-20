package persist

import (
	"slices"
	"sync"
	"time"
)

// Order serializes the writes of one database row against each other, across
// the lanes that carry them.
//
// A lane keeps one owner's writes in order, but a row does not belong to one
// owner for its whole life: an item that changes hands has its next write
// queued on the new owner's lane while the previous owner's is still waiting
// on theirs, and the lazy-persistence tick writes the same row from a lane of
// its own. Two lanes run in parallel, so without something spanning them the
// later write can finish first and the earlier one then overwrites the row
// with what it captured — the wrong owner, a destroyed item's row put back, a
// count from before the last mutation.
//
// Order gives every write a place in its row's order when the write is
// produced, which is the moment its content is decided. Running a write then
// holds that row until the write has finished, and drops it entirely when a
// write reserved later has already landed. A write therefore lands only if
// nothing newer for that row has, and never at the same time as another write
// of the same row.
//
// A nil *Order runs every write unconditionally, for code built without one.
type Order struct {
	mu   sync.Mutex
	rows map[int32]*orderRow
}

// orderRow is one row's ordering state. held is taken for the duration of a
// write, so two writes of one row never overlap; it is a channel rather than
// a mutex so a caller that must not park can give up on it (Write.TryRun).
// next hands out places, applied is the newest place that has finished
// writing, and outstanding counts the reservations still to come, so the
// entry can be dropped once the row goes quiet. next, applied and outstanding
// are guarded by Order.mu.
type orderRow struct {
	held        chan struct{}
	next        uint64
	applied     uint64
	outstanding int
}

// NewOrder returns an empty Order.
func NewOrder() *Order {
	return &Order{rows: make(map[int32]*orderRow)}
}

// Reserve takes ids' places in their rows' write order. Call it on the
// goroutine that decides what the write will contain — for a queued write,
// the actor queue that produced it — and Run the result where the write
// itself happens. Duplicate ids are reserved once.
//
// The reservation has to be taken before whatever serialized the mutation
// lets go of it, not merely before the write is queued: a gap between the two
// lets another actor mutate the same row and reserve inside it, and that
// write would then hold the later place while carrying the older content.
// Today every producer satisfies this because a row's mutation and its
// reservation happen in one actor-queue task — a trade settles both sides and
// reserves without yielding, and a pet runs on its owner's queue.
//
// Every reservation has to be handed back, by Run or by Cancel; a reservation
// that is simply dropped keeps its row's entry alive.
func (o *Order) Reserve(ids ...int32) *Write {
	w := &Write{order: o, ids: slices.Clone(ids)}
	slices.Sort(w.ids)
	w.ids = slices.Compact(w.ids)
	if o == nil {
		return w
	}
	w.places = make([]uint64, len(w.ids))
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.rows == nil {
		o.rows = make(map[int32]*orderRow)
	}
	for i, id := range w.ids {
		r, ok := o.rows[id]
		if !ok {
			r = &orderRow{held: make(chan struct{}, 1)}
			o.rows[id] = r
		}
		r.next++
		r.outstanding++
		w.places[i] = r.next
	}
	return w
}

// Write is one reserved place in each of its rows' write order.
type Write struct {
	order  *Order
	ids    []int32
	places []uint64
}

// Run calls write with the rows still worth writing — those no later write
// has already landed — while holding every row it reserved. write is not
// called at all when none are left. It must write those rows and nothing
// else; ids is in ascending object-id order.
//
// Run waits for those rows and keeps them for as long as write takes, so the
// goroutine calling it is held for the length of the database write, not just
// the bookkeeping. A caller that is a shared drainer — a persist.Worker lane,
// which serves every owner mapped to it — should use TryRun and come back
// rather than parking there while another write's transaction finishes.
//
// Rows are taken in ascending id order, so two batches that overlap cannot
// deadlock against each other.
func (w *Write) Run(write func(ids []int32)) {
	w.run(write, nil)
}

// TryRun is Run for a caller that must not park: it gives up if it cannot
// take every row it reserved within wait, leaving the reservation intact for
// a later Run or TryRun, and reports whether the write was decided. A false
// result means nothing was written and nothing was consumed.
func (w *Write) TryRun(wait time.Duration, write func(ids []int32)) bool {
	return w.run(write, &wait)
}

// Cancel hands the reservation back without writing, for a write that will
// never run — the worker it was queued on has closed. The rows' order is
// unaffected: an unrun place never advances what has been applied.
func (w *Write) Cancel() {
	if w == nil || w.order == nil {
		return
	}
	w.order.forget(w.ids)
}

func (w *Write) run(write func(ids []int32), wait *time.Duration) bool {
	if w == nil {
		return true
	}
	if w.order == nil {
		write(w.ids)
		return true
	}
	rows, ok := w.order.take(w.ids, wait)
	if !ok {
		return false
	}
	// A row is a process-lifetime resource, not a lock a crashing goroutine
	// takes down with it: a panic inside write would otherwise leave it held
	// and every later write of it stuck.
	defer w.order.release(w.ids, rows)
	keep := w.order.keep(w, rows)
	if len(keep) > 0 {
		write(keep)
		w.order.applied(w, rows)
	}
	return true
}

// take holds every row in ids, in ascending id order. With a wait it gives up
// and releases what it already holds once that elapses.
func (o *Order) take(ids []int32, wait *time.Duration) ([]*orderRow, bool) {
	o.mu.Lock()
	rows := make([]*orderRow, len(ids))
	for i, id := range ids {
		rows[i] = o.rows[id]
	}
	o.mu.Unlock()

	var deadline <-chan time.Time
	if wait != nil {
		timer := time.NewTimer(*wait)
		defer timer.Stop()
		deadline = timer.C
	}
	for i, r := range rows {
		select {
		case r.held <- struct{}{}:
		case <-deadline:
			for _, taken := range rows[:i] {
				<-taken.held
			}
			return nil, false
		}
	}
	return rows, true
}

// keep reports which of w's rows no later write has landed on.
func (o *Order) keep(w *Write, rows []*orderRow) []int32 {
	o.mu.Lock()
	defer o.mu.Unlock()
	keep := make([]int32, 0, len(w.ids))
	for i, r := range rows {
		if w.places[i] > r.applied {
			keep = append(keep, w.ids[i])
		}
	}
	return keep
}

// applied records that w's write has landed on the rows it kept.
func (o *Order) applied(w *Write, rows []*orderRow) {
	o.mu.Lock()
	defer o.mu.Unlock()
	for i, r := range rows {
		if w.places[i] > r.applied {
			r.applied = w.places[i]
		}
	}
}

func (o *Order) release(ids []int32, rows []*orderRow) {
	for _, r := range rows {
		<-r.held
	}
	o.forget(ids)
}

// forget drops the reservations ids carried, and the rows left with none.
func (o *Order) forget(ids []int32) {
	o.mu.Lock()
	defer o.mu.Unlock()
	for _, id := range ids {
		r, ok := o.rows[id]
		if !ok {
			continue
		}
		r.outstanding--
		if r.outstanding == 0 {
			delete(o.rows, id)
		}
	}
}
