//go:build !simdebug

package sim

// Without simdebug, AssertOwner relies on q.draining alone.

func enterDrain(*Queue) {}

func exitDrain() {}

func assertDrainer(*Queue) {}
