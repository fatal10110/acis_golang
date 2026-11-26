//go:build !simdebug

package sim

// Without simdebug, AssertOwner relies on q.draining alone and
// AssertNotOnQueue checks nothing.

func enterDrain(*Queue) *Queue { return nil }

func exitDrain(*Queue) {}

func assertDrainer(*Queue) {}

func assertNoDrain() {}
