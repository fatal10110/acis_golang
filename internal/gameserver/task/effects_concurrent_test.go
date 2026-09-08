package task

import (
	"sync"
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

// TestEffectsConcurrentAddRemoveTick exercises the registration path AC #4
// on the tracked issue asks to be proven safe under -race: effect.List.Add
// and Remove (called from arbitrary goroutines whenever a skill lands or
// expires) driving Effects' registry concurrently with Tick, which only
// ever runs from the single scheduler goroutine per Effects' own contract.
func TestEffectsConcurrentAddRemoveTick(t *testing.T) {
	e := NewEffects()
	// NewEffects installs e as the process-wide effect.List activity
	// registrar (effect.SetActivityHook); restore a clean slate so a later
	// test in this package that builds its own effect.List doesn't
	// register into this now-finished test's registry.
	t.Cleanup(func() { effect.SetActivityHook(nil) })

	const listCount = 20
	lists := make([]*effect.List, listCount)
	for i := range lists {
		lists[i] = effect.NewList(benchNoopStatOwner{})
	}
	newEffect := func(id int) *effect.Effect {
		eff, err := effect.New(effect.Skill{ID: modelskill.ID(id)}, modelskill.EffectTemplate{Name: "Buff"})
		if err != nil {
			t.Fatalf("effect.New: %v", err)
		}
		return eff
	}

	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			for j, list := range lists {
				list.Add(newEffect(i*listCount + j + 1))
			}
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			for _, list := range lists {
				for _, eff := range list.All() {
					list.Remove(eff)
				}
			}
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			e.Tick()
		}
	}()
	wg.Wait()

	// Settling phase: the churn above stopped, so each list's registration
	// state must now agree with its actual contents. This is the invariant
	// notifyActivityTransition's atomic decide-then-apply exists to
	// guarantee — a "decide, unlock, apply" version can lose a transition
	// under exactly this kind of concurrent Add/Remove on the same list
	// (Tick draining an expiring effect while a skill lands a new one) and
	// leave a live list unregistered, or an empty one registered, forever.
	for i, list := range lists {
		registered := e.contains(list)
		active := len(list.All()) > 0
		if registered != active {
			t.Errorf("list %d: registered=%v, active=%v (holds %d effects) — registration out of sync with contents", i, registered, active, len(list.All()))
		}
	}
}

// TestEffectsResetClearsRegistrationsAcrossOwners is the regression case
// for gameservertest sharing one Effects instance across sequential test
// servers in a process: a "leftover" list from one owner (an NPC a test
// spawned but never killed) must stop being registered once Reset runs,
// without Reset disturbing an unrelated list that's still legitimately
// active, or leaving that other list unable to register again afterward.
func TestEffectsResetClearsRegistrationsAcrossOwners(t *testing.T) {
	e := NewEffects()
	t.Cleanup(func() { effect.SetActivityHook(nil) })

	newEffect := func(id int) *effect.Effect {
		eff, err := effect.New(effect.Skill{ID: modelskill.ID(id)}, modelskill.EffectTemplate{Name: "Buff"})
		if err != nil {
			t.Fatalf("effect.New: %v", err)
		}
		return eff
	}

	leftover := effect.NewList(benchNoopStatOwner{})
	leftover.Add(newEffect(1))
	if !e.contains(leftover) {
		t.Fatal("leftover list not registered after Add")
	}

	e.Reset()

	if e.contains(leftover) {
		t.Fatal("leftover list still registered after Reset")
	}
	if len(leftover.All()) != 1 {
		t.Fatalf("Reset touched leftover's contents: %d effects, want 1", len(leftover.All()))
	}

	// leftover's owner is still alive in this scenario (Reset only fires
	// because a *different* server tore down; this list's actor keeps
	// playing) and lands another effect. Since leftover was already
	// active going into Reset, this Add doesn't cross an empty->active
	// transition on its own — the regression this guards against is
	// Reset clearing the registry without also clearing List.tracked,
	// which leaves a survivor believing it's still registered and makes
	// every later Add on it a permanent no-op.
	leftover.Add(newEffect(3))
	if !e.contains(leftover) {
		t.Fatal("a list that survived Reset never re-registered on its next Add")
	}

	// A fresh owner (the next test server's own NPC) must still be able to
	// register normally: Reset must not have wedged the hook or the
	// registry into a state that rejects further registrations.
	next := effect.NewList(benchNoopStatOwner{})
	next.Add(newEffect(2))
	if !e.contains(next) {
		t.Fatal("a list added after Reset failed to register")
	}

	// Tick must not panic or otherwise choke on the now-unregistered
	// leftover — it should simply not be visited.
	e.Tick()
}
