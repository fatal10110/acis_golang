package task

import (
	"testing"

	"github.com/rs/zerolog"
)

// contains reports whether actor is the one registered under id.
func (a *AI) contains(id int32, actor AIActor) bool {
	e, ok := a.get(id)
	return ok && e.actor == actor
}

// A tick posts each actor the cycle built when it was added, so a steady
// tick allocates nothing per actor.
func TestAITickAllocatesNothingPerActor(t *testing.T) {
	ai := NewAI(nil, zerolog.Nop())
	actors := []*reusedIDActor{{id: 1}, {id: 2}, {id: 3}}
	for _, a := range actors {
		ai.Add(a)
	}
	cycle := func() {
		if err := ai.Tick(); err != nil {
			t.Fatalf("Tick: %v", err)
		}
		testLoop.Run()
	}
	cycle()

	if allocs := testing.AllocsPerRun(100, cycle); allocs != 0 {
		t.Fatalf("AI tick of %d actors allocates %v times, want 0", len(actors), allocs)
	}
	for _, a := range actors {
		if a.thinks != 102 {
			t.Fatalf("actor %d thought %d times, want 102", a.id, a.thinks)
		}
	}
}
