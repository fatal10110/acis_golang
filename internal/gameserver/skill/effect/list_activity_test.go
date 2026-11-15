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

func (activityTestOwner) AttachStatFuncs([]Mod)       {}
func (activityTestOwner) StatFuncsAttached([]Mod)     {}
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

// TestListActivityFailedStartRegistersUntilFirstTick pins the registration
// of an effect whose start fails: AbortCast's OnStart (abortCastStart)
// returns false when e.Effected is nil, yet the effect stays held, so the
// list registers active. Its first tick ends it, and the list deregisters
// from that settled state.
func TestListActivityFailedStartRegistersUntilFirstTick(t *testing.T) {
	rec := withActivityRecorder(t)
	list := newTestList(activityTestOwner{}, WithEnv(Env{Activity: rec}))

	e, err := New(Skill{ID: 1}, modelskill.EffectTemplate{Name: "AbortCast", Count: 1, Time: 1})
	if err != nil {
		t.Fatal(err)
	}
	// e.Effected is left nil: abortCastStart fails unconditionally.
	list.Add(e)

	if !rec.isActive(list) || !e.InUse() {
		t.Fatalf("list active = %v, effect in use = %v after a failed start, want the effect held", rec.isActive(list), e.InUse())
	}
	list.tickAt(time.Now().Add(time.Hour))
	if rec.isActive(list) || len(list.All()) != 0 {
		t.Fatal("failed-start effect still held after its first tick, want it gone and the list deregistered")
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

func (activityTestOwner) NotifyEffectFelt(modelskill.ID, int) {}

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
