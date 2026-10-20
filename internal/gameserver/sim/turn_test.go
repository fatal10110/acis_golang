package sim

import "testing"

// A turn is held only on the goroutine inside its Run, and only until Run
// returns: another goroutine reading it meanwhile, and the same goroutine
// afterwards, see it unheld.
func TestTurnIsHeldOnlyInsideRunOnItsGoroutine(t *testing.T) {
	var turn Turn
	if turn.Held() {
		t.Fatal("zero Turn held")
	}
	var inside, elsewhere bool
	turn.Run(func() {
		inside = turn.Held()
		done := make(chan bool)
		go func() { done <- turn.Held() }()
		elsewhere = <-done
	})
	if !inside {
		t.Error("Held inside Run = false")
	}
	if elsewhere {
		t.Error("Held on another goroutine while Run runs = true")
	}
	if turn.Held() {
		t.Error("Held after Run returned = true")
	}
}

// A nested Run leaves the turn held for the rest of the outer one, and a
// panic out of Run leaves it unheld.
func TestTurnNestedRunAndPanic(t *testing.T) {
	var turn Turn
	turn.Run(func() {
		turn.Run(func() {})
		if !turn.Held() {
			t.Error("Held after a nested Run returned = false")
		}
	})
	if !panics(func() { turn.Run(func() { panic("hook") }) }) {
		t.Fatal("Run swallowed the panic")
	}
	if turn.Held() {
		t.Error("Held after Run panicked = true")
	}
}
