package creature

import (
	"maps"
	"slices"
	"sync"
	"testing"
)

// recordingSet is a RegenSet that keeps the owners it holds.
type recordingSet struct {
	mu     sync.Mutex
	owners map[any]bool
}

func newRecordingSet() *recordingSet { return &recordingSet{owners: map[any]bool{}} }

func (s *recordingSet) Arm(owner any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.owners[owner] = true
}

func (s *recordingSet) Disarm(owner any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.owners, owner)
}

func (s *recordingSet) held() []any {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Collect(maps.Keys(s.owners))
}

// TestRegenBoundSetFollowsArming pins the set membership a bound phase
// reports: it joins on the arming settle, stays through later settles and
// claimed ticks, and leaves once a settle disarms it.
func TestRegenBoundSetFollowsArming(t *testing.T) {
	t.Parallel()
	f := newRegenFixture()
	set := newRecordingSet()
	f.regen.Bind(set, f)
	if got := set.held(); len(got) != 0 {
		t.Fatalf("idle phase bound: set holds %v, want nothing", got)
	}

	f.short = true
	f.settle()
	if got := set.held(); len(got) != 1 || got[0] != f {
		t.Fatalf("armed phase: set holds %v, want its owner", got)
	}
	f.settle()
	if !f.claim(RegenPeriod) {
		t.Fatal("tick not claimed one period after the drop")
	}
	if got := set.held(); len(got) != 1 {
		t.Fatalf("after a repeat settle and a claimed tick: set holds %v, want the owner", got)
	}

	f.short = false
	f.settle()
	if got := set.held(); len(got) != 0 {
		t.Fatalf("disarmed phase: set holds %v, want nothing", got)
	}
}

// TestRegenBindArmedPhase covers a phase armed before it is bound: binding
// adds it at once, binding again changes nothing, binding elsewhere moves
// it, and unbinding takes it out while the phase itself stays armed.
func TestRegenBindArmedPhase(t *testing.T) {
	t.Parallel()
	f := newRegenFixture()
	f.short = true
	f.settle()

	first, second := newRecordingSet(), newRecordingSet()
	f.regen.Bind(first, f)
	f.regen.Bind(first, f)
	if got := first.held(); len(got) != 1 {
		t.Fatalf("armed phase bound: set holds %v, want its owner", got)
	}

	f.regen.Bind(second, f)
	if got := first.held(); len(got) != 0 {
		t.Fatalf("rebound phase: old set holds %v, want nothing", got)
	}
	if got := second.held(); len(got) != 1 {
		t.Fatalf("rebound phase: new set holds %v, want its owner", got)
	}

	f.regen.Unbind(first) // not its set: ignored
	if got := second.held(); len(got) != 1 {
		t.Fatalf("unbind from another set: set holds %v, want the owner", got)
	}
	f.regen.Unbind(second)
	if got := second.held(); len(got) != 0 {
		t.Fatalf("unbound phase: set holds %v, want nothing", got)
	}
	if !f.regen.Active() {
		t.Fatal("unbinding disarmed the phase")
	}
	f.settle()
	if got := second.held(); len(got) != 0 {
		t.Fatalf("settle after unbinding: set holds %v, want nothing", got)
	}
}
