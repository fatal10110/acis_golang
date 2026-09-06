package effect

import (
	"sync"
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// activityTestOwner satisfies StatOwner without recording anything, for
// tests that only care about a List's registration state, not its owner
// side effects.
type activityTestOwner struct{}

func (activityTestOwner) AddStatFuncs([]Mod)          {}
func (activityTestOwner) RemoveStatsByOwner(ModOwner) {}
func (activityTestOwner) MaxBuffCount() int           { return 20 }

// activityRecorder is a minimal fake of task.Effects' registration side:
// it just remembers whether each list is currently considered active, so
// tests can assert the registration state machine without pulling in the
// task package.
type activityRecorder struct {
	mu     sync.Mutex
	active map[*List]bool
}

func newActivityRecorder() *activityRecorder {
	return &activityRecorder{active: make(map[*List]bool)}
}

func (r *activityRecorder) track(list *List, active bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if active {
		r.active[list] = true
	} else {
		delete(r.active, list)
	}
}

func (r *activityRecorder) isActive(list *List) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.active[list]
}

// withActivityRecorder installs rec as the process-wide activity hook for
// the duration of the test and restores the previous hook (nil, since no
// production code installs one during `go test ./internal/gameserver/skill/effect/...`)
// on cleanup.
func withActivityRecorder(t *testing.T) *activityRecorder {
	t.Helper()
	rec := newActivityRecorder()
	SetActivityHook(rec.track)
	t.Cleanup(func() { SetActivityHook(nil) })
	return rec
}

func mustNewEffect(t *testing.T, skill Skill, name string) *Effect {
	t.Helper()
	e, err := New(skill, modelskill.EffectTemplate{Name: name})
	if err != nil {
		t.Fatalf("New(%q): %v", name, err)
	}
	return e
}

func TestListActivityRegistersOnFirstEffect(t *testing.T) {
	rec := withActivityRecorder(t)
	list := NewList(activityTestOwner{})

	if rec.isActive(list) {
		t.Fatal("empty list reported active before any Add")
	}

	list.Add(mustNewEffect(t, Skill{ID: 1}, "Buff"))

	if !rec.isActive(list) {
		t.Fatal("list not registered active after its first effect landed")
	}
}

func TestListActivityDeregistersOnLastRemove(t *testing.T) {
	rec := withActivityRecorder(t)
	list := NewList(activityTestOwner{})

	e := mustNewEffect(t, Skill{ID: 1}, "Buff")
	list.Add(e)
	if !rec.isActive(list) {
		t.Fatal("list not registered active after Add")
	}

	list.Remove(e)
	if rec.isActive(list) {
		t.Fatal("list still registered active after its only effect was removed")
	}
}

// TestListActivityStaysRegisteredWithEffectsRemaining guards against a hook
// that deregisters on any Remove instead of only the one draining the list
// to empty.
func TestListActivityStaysRegisteredWithEffectsRemaining(t *testing.T) {
	rec := withActivityRecorder(t)
	list := NewList(activityTestOwner{})

	e1 := mustNewEffect(t, Skill{ID: 1}, "Buff")
	e2 := mustNewEffect(t, Skill{ID: 2}, "Buff")
	list.Add(e1)
	list.Add(e2)

	list.Remove(e1)

	if !rec.isActive(list) {
		t.Fatal("list deregistered while a second effect is still held")
	}
}

// TestListActivityRejectedOnStartNeverRegisters is the regression case for
// firing the activity hook from a pre-runHooks snapshot: AbortCast's
// OnStart (abortCastStart) deterministically returns false when e.Effected
// is nil, draining the list back to empty inside runHooks before Add
// returns. The hook must reflect that settled state, not the transient
// "just inserted" one, or a resisted effect leaves a permanent phantom
// registration that Effects.Tick then scans forever.
func TestListActivityRejectedOnStartNeverRegisters(t *testing.T) {
	rec := withActivityRecorder(t)
	list := NewList(activityTestOwner{})

	e := mustNewEffect(t, Skill{ID: 1}, "AbortCast")
	// e.Effected is left nil: abortCastStart rejects unconditionally.
	list.Add(e)

	if rec.isActive(list) {
		t.Fatal("list registered active after an OnStart-rejected effect drained it back to empty")
	}
}

// TestListUntrackDeregistersRegardlessOfContents is the regression case for
// the actor-teardown leak: a list that still holds a live effect must stop
// ticking once its owner leaves the world for good.
func TestListUntrackDeregistersRegardlessOfContents(t *testing.T) {
	rec := withActivityRecorder(t)
	list := NewList(activityTestOwner{})
	list.Add(mustNewEffect(t, Skill{ID: 1}, "Buff"))

	if !rec.isActive(list) {
		t.Fatal("list not registered active after Add")
	}

	list.Untrack()

	if rec.isActive(list) {
		t.Fatal("list still registered active after Untrack")
	}
}
