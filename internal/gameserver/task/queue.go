package task

import "github.com/fatal10110/acis_golang/internal/gameserver/sim"

// Queued is the part of every actor a task ticks that says where the actor's
// work runs. A tick posts each actor's share of the work to its queue
// instead of running it on the ticker goroutine; registries keep their own
// lock for membership.
type Queued interface {
	// Queue returns the actor's queue, or nil for an actor built without
	// one, whose work then runs on the ticking goroutine.
	Queue() *sim.Queue
}

// post runs fn as a task on q, or right away when q is nil. A closed queue
// belongs to an actor that has left the world, so fn is dropped.
func post(q *sim.Queue, fn func()) {
	if q == nil {
		fn()
		return
	}
	q.Post(fn)
}
