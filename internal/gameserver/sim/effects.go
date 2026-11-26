package sim

// Effects are calls a change defers until its lock is released, run in
// the order they were added. Each owner decides when and on which
// goroutine its Effects run.
type Effects []func()

// Add queues fn after the calls already queued.
func (fx *Effects) Add(fn func()) { *fx = append(*fx, fn) }

// Run makes the queued calls in order.
func (fx Effects) Run() {
	for _, fn := range fx {
		fn()
	}
}
