package ai

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// hookRecorder records the hook points a fakeActor reached, in order.
type hookRecorder struct {
	points []HookPoint
	// at, when set, runs inside every hook point.
	at func(HookPoint)
}

func (a *fakeActor) AtHookPoint(p HookPoint) {
	a.hooks.points = append(a.hooks.points, p)
	if a.hooks.at != nil {
		a.hooks.at(p)
	}
}

// assertBrainUnlocked fails unless brain's mutex is free at hook point p.
func assertBrainUnlocked(t *testing.T, brain *Attackable, p HookPoint) {
	t.Helper()
	if !brain.mu.TryLock() {
		t.Fatalf("hook point %d ran with the brain mutex held", p)
	}
	brain.mu.Unlock()
}

func assertHookPoints(t *testing.T, owner *fakeActor, what string, want ...HookPoint) {
	t.Helper()
	if !slices.Equal(owner.hooks.points, want) {
		t.Fatalf("hook points %s = %v, want %v", what, owner.hooks.points, want)
	}
}

// TestHookPointNoDesireClosesTheIdle pins the no-desire point at the end of
// the idle: after the abort, the walk stance and the switch to idle, before
// the idle wander is queued, with the brain mutex released.
func TestHookPointNoDesireClosesTheIdle(t *testing.T) {
	owner := actor(1)
	owner.idleWander = true
	move := &recordingMove{}
	strike := &recordingAttack{}
	brain := NewAttackable(owner, move, strike)
	owner.hooks.at = func(p HookPoint) {
		if p != HookNoDesire {
			return
		}
		assertBrainUnlocked(t, brain, p)
		if move.stopCount == 0 || strike.stopCalls == 0 || owner.walkStanceCalls == 0 {
			t.Fatalf("no-desire point before the idle abort: move stops %d, attack stops %d, walk stance %d",
				move.stopCount, strike.stopCalls, owner.walkStanceCalls)
		}
		if got := brain.CurrentIntention(); got != IntentionIdle {
			t.Fatalf("CurrentIntention() at the no-desire point = %v, want idle", got)
		}
		if got := brain.Desires().Len(); got != 0 {
			t.Fatalf("queued desires at the no-desire point = %d, want the idle wander still unqueued", got)
		}
	}

	if err := tickThinkIdle(brain); err != nil {
		t.Fatalf("TickThink() error: %v", err)
	}
	assertHookPoints(t, owner, "after two cycles", HookSeeCreature, HookSeeCreature, HookNoDesire)
	if !brain.Desires().Has(&Desire{Kind: IntentionWander}) {
		t.Fatal("idle wander not queued after the no-desire point")
	}
}

// TestHookPointNoDesireOnEventIdle pins the no-desire point on desire
// selection's own empty-queue idle, which opens no see-creature point.
func TestHookPointNoDesireOnEventIdle(t *testing.T) {
	owner := actor(1)
	owner.x = 100
	brain := NewAttackable(owner, &recordingMove{}, &recordingAttack{})
	if err := brain.TickThink(); err != nil {
		t.Fatalf("first TickThink() error: %v", err)
	}
	brain.AddMoveToDesire(location.Location{X: 300}, 1_000)
	if err := brain.RunAI(); err != nil {
		t.Fatalf("RunAI() error: %v", err)
	}
	brain.Desires().Clear()
	owner.hooks.points = nil
	owner.hooks.at = func(p HookPoint) { assertBrainUnlocked(t, brain, p) }

	if err := brain.RunAI(); err != nil {
		t.Fatalf("idling RunAI() error: %v", err)
	}
	assertHookPoints(t, owner, "on the event idle", HookNoDesire)
}

// TestHookPointSeeCreatureOpensOnlyThePeriodicCycle pins the see-creature
// point at the start of the periodic cycle, ahead of desire selection: a
// desire queued there is taken up by the same cycle. Event and continue
// passes open no point.
func TestHookPointSeeCreatureOpensOnlyThePeriodicCycle(t *testing.T) {
	owner := actor(1)
	owner.x = 100
	move := &recordingMove{}
	brain := NewAttackable(owner, move, &recordingAttack{})
	if err := brain.TickThink(); err != nil {
		t.Fatalf("first TickThink() error: %v", err)
	}
	for name, pass := range map[string]func() error{
		"RunAI": brain.RunAI, "Think": brain.Think, "AttackFinished": brain.AttackFinished,
	} {
		owner.hooks.points = nil
		if err := pass(); err != nil {
			t.Fatalf("%s() error: %v", name, err)
		}
		assertHookPoints(t, owner, "on "+name)
	}

	dest := location.Location{X: 300}
	owner.hooks.points = nil
	owner.hooks.at = func(p HookPoint) {
		assertBrainUnlocked(t, brain, p)
		if p == HookSeeCreature {
			brain.AddMoveToDesire(dest, 1_000)
		}
	}
	if err := brain.TickThink(); err != nil {
		t.Fatalf("TickThink() error: %v", err)
	}
	assertHookPoints(t, owner, "on the periodic cycle", HookSeeCreature)
	if got := brain.CurrentIntention(); got != IntentionMoveTo || move.home != dest {
		t.Fatalf("after a walk queued at the see-creature point: intention %v walking to %+v, want %v to %+v",
			got, move.home, IntentionMoveTo, dest)
	}
}

// TestHookPointNestedThinkRunsInPlace pins a think raised from inside a hook
// point: it runs to its end before the point returns. The cycle around it
// then reads the state the nested think left, while its first-cycle
// promotion gate, decided before the point, stays shut.
func TestHookPointNestedThinkRunsInPlace(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	strike := &recordingAttack{canAttack: true}
	brain := NewAttackable(owner, &recordingMove{}, strike)
	owner.hooks.at = func(p HookPoint) {
		if p != HookSeeCreature {
			return
		}
		brain.AddAttackDesire(target, 100)
		if strike.doAttackCalls != 1 {
			t.Fatalf("swings when the nested think returned = %d, want 1", strike.doAttackCalls)
		}
	}

	if err := brain.TickThink(); err != nil {
		t.Fatalf("TickThink() error: %v", err)
	}
	if strike.doAttackCalls != 1 {
		t.Fatalf("swings after the cycle = %d, want only the nested think's", strike.doAttackCalls)
	}
	if got := brain.CurrentIntention(); got != IntentionAttack {
		t.Fatalf("CurrentIntention() after the cycle = %v, want %v", got, IntentionAttack)
	}
}

// TestHookPointLetsAnotherQueueThink pins that a think raised on another
// goroutine while a hook point is open runs at once instead of waiting for
// the cycle to finish.
func TestHookPointLetsAnotherQueueThink(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	strike := &recordingAttack{canAttack: true}
	brain := NewAttackable(owner, &recordingMove{}, strike)
	open, done := make(chan struct{}), make(chan struct{})
	go func() {
		<-open
		brain.AddAttackDesire(target, 100)
		close(done)
	}()
	owner.hooks.at = func(p HookPoint) {
		if p != HookSeeCreature {
			return
		}
		close(open)
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Fatal("a think on another goroutine waited for the open hook point")
		}
	}

	if err := brain.TickThink(); err != nil {
		t.Fatalf("TickThink() error: %v", err)
	}
	if strike.doAttackCalls != 1 {
		t.Fatalf("swings = %d, want the other goroutine's one", strike.doAttackCalls)
	}
}

// TestHookPointMoveFinishedPrecedesDesireDrop pins the move-finished point
// on a walk's arrival: it runs before the walk's desire is dropped, only for
// a walk, and not on a blocked arrival.
func TestHookPointMoveFinishedPrecedesDesireDrop(t *testing.T) {
	owner := actor(1)
	owner.x = 100
	brain := NewAttackable(owner, &recordingMove{}, &recordingAttack{})
	dest := location.Location{X: 300}
	walk := &Desire{Kind: IntentionMoveTo, Location: dest}
	brain.AddMoveToDesire(dest, 1_000)
	if err := brain.RunAI(); err != nil {
		t.Fatalf("RunAI() error: %v", err)
	}
	owner.hooks.at = func(p HookPoint) {
		assertBrainUnlocked(t, brain, p)
		if p == HookMoveFinished && !brain.Desires().Has(walk) {
			t.Fatal("walk desire dropped before the move-finished point")
		}
	}

	brain.ArrivedBlocked()
	assertHookPoints(t, owner, "on a blocked arrival")
	brain.AddMoveToDesire(dest, 1_000)

	brain.Arrived()
	assertHookPoints(t, owner, "on the walk's arrival", HookMoveFinished)
	if brain.Desires().Has(walk) {
		t.Fatal("walk desire kept after the arrival")
	}

	owner.hooks.points = nil
	if err := thinkWanderOnce(brain); err != nil {
		t.Fatalf("wander RunAI() error: %v", err)
	}
	brain.Arrived()
	assertHookPoints(t, owner, "on a wander's arrival")
}

// TestHookPointMoveFinishedAtDestination pins the move-finished point on a
// walk step that finds the actor already at its destination.
func TestHookPointMoveFinishedAtDestination(t *testing.T) {
	owner := actor(1)
	owner.x = 100
	brain := NewAttackable(owner, &recordingMove{}, &recordingAttack{})
	here := location.Location{X: 100}
	owner.hooks.at = func(p HookPoint) {
		assertBrainUnlocked(t, brain, p)
		if !brain.Desires().Has(&Desire{Kind: IntentionMoveTo, Location: here}) {
			t.Fatal("walk desire dropped before the move-finished point")
		}
	}
	brain.AddMoveToDesire(here, 1_000)

	if err := brain.RunAI(); err != nil {
		t.Fatalf("RunAI() error: %v", err)
	}
	assertHookPoints(t, owner, "on a walk at its destination", HookMoveFinished)
	if got := brain.Desires().Len(); got != 0 {
		t.Fatalf("queued desires after the walk = %d, want its desire dropped", got)
	}
}

// TestHookPointOutOfTerritoryOnFirstOutsideArrival pins the out-of-territory
// point: on the first arrival outside territory, after the move-finished
// point and before the stale-hate sweep is armed; again only after an
// arrival back inside.
func TestHookPointOutOfTerritoryOnFirstOutsideArrival(t *testing.T) {
	owner := actor(1)
	owner.x = 100
	owner.inTerritory = false
	brain := NewAttackable(owner, &recordingMove{}, &recordingAttack{})
	owner.hooks.at = func(p HookPoint) {
		assertBrainUnlocked(t, brain, p)
		if p != HookOutOfTerritory {
			return
		}
		brain.mu.Lock()
		armed := brain.ootSweep
		brain.mu.Unlock()
		if armed {
			t.Fatal("stale-hate sweep armed before the out-of-territory point")
		}
	}
	dest := location.Location{X: 300}
	brain.AddMoveToDesire(dest, 1_000)
	if err := brain.RunAI(); err != nil {
		t.Fatalf("RunAI() error: %v", err)
	}

	brain.Arrived()
	assertHookPoints(t, owner, "on the first outside arrival", HookMoveFinished, HookOutOfTerritory)

	// Idle, so later arrivals finish no walk.
	brain.SetBackToPeace()
	owner.hooks.points = nil
	brain.Arrived()
	assertHookPoints(t, owner, "on a second outside arrival")

	owner.inTerritory = true
	brain.Arrived()
	owner.inTerritory = false
	brain.Arrived()
	assertHookPoints(t, owner, "after an arrival back inside", HookOutOfTerritory)
}
