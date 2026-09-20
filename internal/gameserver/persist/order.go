package persist

import (
	"slices"
	"sync"
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

// orderRow is one row's ordering state. mu is held for the duration of a
// write, so two writes of one row never overlap; next hands out places,
// applied is the newest place that has finished writing, and held counts the
// reservations still outstanding so the entry can be dropped when the row
// goes quiet.
type orderRow struct {
	mu      sync.Mutex
	next    uint64
	applied uint64
	held    int
}

// NewOrder returns an empty Order.
func NewOrder() *Order {
	return &Order{rows: make(map[int32]*orderRow)}
}

// Reserve takes ids' places in their rows' write order. Call it on the
// goroutine that decides what the write will contain — for a queued write,
// the actor queue that produced it — and Run the result where the write
// itself happens. Duplicate ids are reserved once.
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
			r = &orderRow{}
			o.rows[id] = r
		}
		r.next++
		r.held++
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
// Rows are locked in that same order, so two batches that overlap cannot
// deadlock against each other.
func (w *Write) Run(write func(ids []int32)) {
	if w == nil {
		return
	}
	if w.order == nil {
		write(w.ids)
		return
	}
	rows := w.order.lock(w.ids)
	keep := make([]int32, 0, len(w.ids))
	for i, r := range rows {
		if w.places[i] > r.applied {
			keep = append(keep, w.ids[i])
		}
	}
	if len(keep) > 0 {
		write(keep)
		for i, r := range rows {
			if w.places[i] > r.applied {
				r.applied = w.places[i]
			}
		}
	}
	w.order.release(w.ids, rows)
}

func (o *Order) lock(ids []int32) []*orderRow {
	o.mu.Lock()
	rows := make([]*orderRow, len(ids))
	for i, id := range ids {
		rows[i] = o.rows[id]
	}
	o.mu.Unlock()
	for _, r := range rows {
		r.mu.Lock()
	}
	return rows
}

func (o *Order) release(ids []int32, rows []*orderRow) {
	for _, r := range rows {
		r.mu.Unlock()
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	for i, r := range rows {
		r.held--
		if r.held == 0 {
			delete(o.rows, ids[i])
		}
	}
}
