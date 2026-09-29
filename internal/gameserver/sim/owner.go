//go:build !simdebug

package sim

// Without simdebug, AssertOwner relies on q.draining alone.

func enterDrain(*Queue) *Queue { return nil }

func exitDrain(*Queue) {}

func assertDrainer(*Queue) {}
