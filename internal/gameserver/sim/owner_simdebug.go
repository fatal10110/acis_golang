//go:build simdebug

package sim

import "sync"

// drainers maps a goroutine id to the queue it is draining.
var drainers sync.Map

// enterDrain records q as the queue the calling goroutine drains and returns
// the one it drained before, which a nested RunOwned restores on exit.
func enterDrain(q *Queue) *Queue {
	prev, _ := drainers.Swap(goid(), q)
	outer, _ := prev.(*Queue)
	return outer
}

func exitDrain(outer *Queue) {
	if outer == nil {
		drainers.Delete(goid())
		return
	}
	drainers.Store(goid(), outer)
}

func assertDrainer(q *Queue) {
	if owner, _ := drainers.Load(goid()); owner != q {
		panic("sim: state of queue " + q.id + " touched while another goroutine drains it")
	}
}
