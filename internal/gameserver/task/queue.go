package task

import "github.com/fatal10110/acis_golang/internal/gameserver/sim"

// Queued is the part of every actor a task ticks that says where the actor's
// work runs. A tick posts each actor's share of the work to its queue
// instead of running it on the ticker goroutine; registries keep their own
// lock for membership.
type Queued interface {
	// Queue returns the actor's queue. Every live actor has one.
	Queue() *sim.Queue
}
