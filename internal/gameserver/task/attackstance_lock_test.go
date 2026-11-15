package task

import (
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
)

// heldLocker is a mutex that reports whether it is held.
type heldLocker struct {
	mu   sync.Mutex
	held bool
}

func (l *heldLocker) Lock()   { l.mu.Lock(); l.held = true }
func (l *heldLocker) Unlock() { l.held = false; l.mu.Unlock() }

// attackStanceLockedActor is an actor whose stance is also refreshed off its
// queue, under its own stance lock. It records whether that lock was held
// when its in-combat flag was cleared.
type attackStanceLockedActor struct {
	attackStanceFakeActor
	lock          heldLocker
	clearedLocked []bool
}

func (a *attackStanceLockedActor) StanceLock() sync.Locker { return &a.lock }

func (a *attackStanceLockedActor) SetInCombat(inCombat bool) bool {
	if !inCombat {
		a.clearedLocked = append(a.clearedLocked, a.lock.held)
	}
	return a.attackStanceFakeActor.SetInCombat(inCombat)
}

// lockCheckingEffects records whether the actor's stance lock was held when
// its AutoAttackStop went out.
type lockCheckingEffects struct {
	actor       *attackStanceLockedActor
	stopsLocked []bool
}

func (e *lockCheckingEffects) AutoAttackStop(AttackStanceActor) {
	e.stopsLocked = append(e.stopsLocked, e.actor.lock.held)
}

// TestAttackStanceExpiryHoldsActorStanceLock pins that a queued expiry runs
// its deadline check, in-combat flag clear and AutoAttackStop under the
// actor's stance lock, the one a refresh from another goroutine holds, so
// the refresh cannot land between them.
func TestAttackStanceExpiryHoldsActorStanceLock(t *testing.T) {
	inline := sim.NewInline(time.UnixMilli(0))
	base := time.UnixMilli(0)
	now := base
	actor := &attackStanceLockedActor{attackStanceFakeActor: attackStanceFakeActor{id: 1, inCombat: true}}
	actor.queue = inline.NewQueue("actor-1")
	effects := &lockCheckingEffects{actor: actor}
	stance, err := NewAttackStance(effects, func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewAttackStance() error = %v", err)
	}
	stance.Add(actor)

	now = base.Add(AttackStancePeriod + time.Second)
	if err := stance.Tick(); err != nil {
		t.Fatalf("Tick() = %v", err)
	}
	inline.Run()

	if len(actor.clearedLocked) != 1 || !actor.clearedLocked[0] {
		t.Errorf("in-combat flag clears, lock held = %v, want one under the lock", actor.clearedLocked)
	}
	if len(effects.stopsLocked) != 1 || !effects.stopsLocked[0] {
		t.Errorf("AutoAttackStop calls, lock held = %v, want one under the lock", effects.stopsLocked)
	}
	if stance.InAttackStance(actor) {
		t.Error("expired stance still tracked")
	}
}
