package effect

import (
	"sync"
	"testing"
	"time"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
)

// newTestList returns a list whose owner runs on its own inline queue,
// with the clock reading the wall time at creation.
func newTestList(owner StatOwner, opts ...Option) *List {
	l := NewList(owner, opts...)
	l.SetQueue(sim.NewInline(time.Now()).NewQueue("test"))
	return l
}

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

func (r *activityRecorder) SetActive(list *List, active bool) {
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

func withActivityRecorder(t *testing.T) *activityRecorder {
	t.Helper()
	return newActivityRecorder()
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
	list := newTestList(activityTestOwner{}, WithEnv(Env{Activity: rec}))

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
	list := newTestList(activityTestOwner{}, WithEnv(Env{Activity: rec}))

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
	list := newTestList(activityTestOwner{}, WithEnv(Env{Activity: rec}))

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
	list := newTestList(activityTestOwner{}, WithEnv(Env{Activity: rec}))

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
	list := newTestList(activityTestOwner{}, WithEnv(Env{Activity: rec}))
	list.Add(mustNewEffect(t, Skill{ID: 1}, "Buff"))

	if !rec.isActive(list) {
		t.Fatal("list not registered active after Add")
	}

	list.Untrack()

	if rec.isActive(list) {
		t.Fatal("list still registered active after Untrack")
	}
}

func (activityTestOwner) NotifyEffectAborted(modelskill.ID, int) {}

func (activityTestOwner) NotifyEffectDisappeared(modelskill.ID, int) {}

func (activityTestOwner) NotifyEffectWornOff(modelskill.ID, int) {}

func (activityTestOwner) UpdateEffectIcons() {}

type nightStub bool

func (n nightStub) IsNight() bool { return bool(n) }

func TestListIsNightReadsEnvSource(t *testing.T) {
	if NewList(nil).IsNight() {
		t.Fatal("IsNight() = true with no source, want day")
	}
	if !NewList(nil, WithEnv(Env{Night: nightStub(true)})).IsNight() {
		t.Fatal("IsNight() = false with a night source")
	}
	if NewList(nil, WithEnv(Env{Night: nightStub(false)})).IsNight() {
		t.Fatal("IsNight() = true with a day source")
	}
}
