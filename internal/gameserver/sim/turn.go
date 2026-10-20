package sim

import "sync/atomic"

// Turn marks the goroutine running a stretch of one queue's task, so that
// code reached with a value from that task can tell, on any goroutine,
// whether it still runs inside that stretch: a value handed out of the task
// (stored where another queue reads it, or captured by a closure another
// queue runs) reports false there, and false again on the same goroutine
// once the stretch is over.
//
// Run is called only from tasks of one queue, so no two goroutines run it
// at once; Held may be called from anywhere. The zero Turn is held by no
// goroutine.
type Turn struct {
	// holder is the id of the goroutine inside Run, 0 for none.
	holder atomic.Uint64
}

// Run runs fn on the calling goroutine as the turn's holder: Held reports
// true on this goroutine until fn returns or panics. A nested Run on the
// same goroutine keeps the turn held after it returns.
func (t *Turn) Run(fn func()) {
	prev := t.holder.Swap(goid())
	defer t.holder.Store(prev)
	fn()
}

// Held reports whether the caller runs inside Run of t.
func (t *Turn) Held() bool {
	h := t.holder.Load()
	return h != 0 && h == goid()
}
