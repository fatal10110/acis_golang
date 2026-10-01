package ai

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

func tickThinkPromote(ai *Attackable) error {
	if err := tickThinkIdle(ai); err != nil {
		return err
	}
	return ai.TickThink()
}

type followStub struct {
	*fakeActor
	idleTarget    attackable.Combatant
	thinkCalls    int
	lastWasFollow bool
}

func (f *followStub) IdleFollowTarget() attackable.Combatant { return f.idleTarget }

func (f *followStub) ThinkFollow(target attackable.Combatant, lastWasFollow bool) bool {
	f.thinkCalls++
	f.lastWasFollow = lastWasFollow
	return false
}

func TestAttackableIdleFollowPromotesOnThink(t *testing.T) {
	owner := &followStub{fakeActor: actor(1), idleTarget: actor(9)}
	brain := NewAttackable(owner, &recordingMove{}, &recordingAttack{})

	if err := tickThinkPromote(brain); err != nil {
		t.Fatalf("TickThink() error: %v", err)
	}
	if got := brain.CurrentIntention(); got != IntentionFollow {
		t.Fatalf("CurrentIntention() = %v, want %v", got, IntentionFollow)
	}
	if owner.thinkCalls != 0 {
		t.Fatalf("ThinkFollow calls on the promoting pass = %d, want 0 (even AI step skips)", owner.thinkCalls)
	}
}

// followTickCycles promotes an escort FOLLOW, then runs six periodic AI
// cycles (Tick then TickThink, as the AI task does) and reports which
// cycles, 1-based, stepped the follow. between runs after every cycle.
func followTickCycles(t *testing.T, between func(*Attackable)) []int {
	t.Helper()
	owner := &followStub{fakeActor: actor(1), idleTarget: actor(9)}
	brain := NewAttackable(owner, &recordingMove{}, &recordingAttack{})
	if err := tickThinkPromote(brain); err != nil {
		t.Fatalf("TickThink() error: %v", err)
	}
	if got := brain.CurrentIntention(); got != IntentionFollow {
		t.Fatalf("CurrentIntention() = %v, want %v", got, IntentionFollow)
	}
	var moved []int
	for cycle := 1; cycle <= 6; cycle++ {
		before := owner.thinkCalls
		brain.Tick()
		if err := brain.TickThink(); err != nil {
			t.Fatalf("cycle %d TickThink() error: %v", cycle, err)
		}
		if got := brain.CurrentIntention(); got != IntentionFollow {
			t.Fatalf("cycle %d CurrentIntention() = %v, want %v", cycle, got, IntentionFollow)
		}
		if owner.thinkCalls > before {
			moved = append(moved, cycle)
		}
		between(brain)
	}
	return moved
}

// The escort FOLLOW step reads the AI step counter as the periodic cycle
// saw it before advancing it (0, 1, 2, then reset), and moves only on an
// odd value: once every three cycles. An extra Think between cycles reads
// the counter without advancing it, so the cycles that move do not shift.
func TestAttackableEscortFollowCadenceIgnoresExtraThink(t *testing.T) {
	want := []int{2, 5}
	if got := followTickCycles(t, func(*Attackable) {}); !slices.Equal(got, want) {
		t.Fatalf("follow cycles without extra Think = %v, want %v", got, want)
	}
	extra := func(brain *Attackable) {
		if err := brain.Think(); err != nil {
			t.Fatalf("Think() error: %v", err)
		}
	}
	if got := followTickCycles(t, extra); !slices.Equal(got, want) {
		t.Fatalf("follow cycles with an extra Think after each cycle = %v, want %v", got, want)
	}
}

func TestAttackableAttackDesireReplacesFollow(t *testing.T) {
	owner := &followStub{fakeActor: actor(1), idleTarget: actor(9)}
	target := actor(2)
	owner.known[target.ObjectID()] = true
	strike := &recordingAttack{canAttack: true}
	brain := NewAttackable(owner, &recordingMove{}, strike)

	if err := tickThinkPromote(brain); err != nil {
		t.Fatalf("TickThink() error: %v", err)
	}
	if got := brain.CurrentIntention(); got != IntentionFollow {
		t.Fatalf("CurrentIntention() after idle = %v, want %v", got, IntentionFollow)
	}

	brain.AddDamageHate(target, 0, 200)
	brain.AddAttackDesire(target, 200)
	brain.Think()
	if got := brain.CurrentIntention(); got != IntentionFollow {
		t.Fatalf("CurrentIntention() after Think = %v, want %v (Think does not select)", got, IntentionFollow)
	}
	if strike.target != nil {
		t.Fatalf("Think attacked %v, want the attack left for RunAI", strike.target)
	}
	brain.RunAI()
	if got := brain.CurrentIntention(); got != IntentionAttack {
		t.Fatalf("CurrentIntention() after attack desire = %v, want %v", got, IntentionAttack)
	}
	if strike.target != target {
		t.Fatalf("attack target = %v, want the queued attacker", strike.target)
	}
}

// latchedAttackAI returns an AI past its first periodic cycle that promoted
// an attack on target out of idle through RunAI, so the attack is latched,
// then lost that attack desire while keeping its hate.
func latchedAttackAI(t *testing.T) (*Attackable, *fakeActor, *recordingAttack) {
	t.Helper()
	owner := actor(1)
	owner.idleWander = true
	target := actor(2)
	owner.known = map[int32]bool{target.ObjectID(): true}
	strike := &recordingAttack{canAttack: true}
	a := NewAttackable(owner, &recordingMove{}, strike)
	if err := a.TickThink(); err != nil {
		t.Fatalf("spawn-cycle TickThink() error: %v", err)
	}
	addAttackHate(a, target, 0, 20)
	if err := a.RunAI(); err != nil {
		t.Fatalf("RunAI() error: %v", err)
	}
	if strike.doAttackCalls != 1 {
		t.Fatalf("DoAttack calls after promotion = %d, want 1", strike.doAttackCalls)
	}
	a.Desires().Remove(IntentionAttack, target)
	return a, owner, strike
}

func TestAttackableTickRunsLatchedAttackThenIdles(t *testing.T) {
	a, _, strike := latchedAttackAI(t)
	stops := strike.stopCalls

	if err := a.TickThink(); err != nil {
		t.Fatalf("TickThink() error: %v", err)
	}

	if strike.doAttackCalls != 2 {
		t.Fatalf("DoAttack calls after the latched tick = %d, want 2", strike.doAttackCalls)
	}
	if strike.stopCalls <= stops {
		t.Fatal("latched tick did not abort the attack for the empty-queue idle")
	}
	if got := a.CurrentIntention(); got != IntentionIdle {
		t.Fatalf("CurrentIntention() = %v, want %v", got, IntentionIdle)
	}
	if !a.Desires().Has(&Desire{Kind: IntentionWander}) {
		t.Fatal("wander desire missing after the latched tick's idle")
	}
}

func TestAttackableThinkNeitherRunsNorClearsLatch(t *testing.T) {
	a, _, strike := latchedAttackAI(t)

	if err := a.Think(); err != nil {
		t.Fatalf("Think() error: %v", err)
	}
	if strike.doAttackCalls != 1 {
		t.Fatalf("DoAttack calls after Think = %d, want 1", strike.doAttackCalls)
	}

	if err := a.RunAI(); err != nil {
		t.Fatalf("RunAI() error: %v", err)
	}
	if strike.doAttackCalls != 2 {
		t.Fatalf("DoAttack calls after the latched RunAI = %d, want 2", strike.doAttackCalls)
	}
	if err := a.RunAI(); err != nil {
		t.Fatalf("RunAI() error: %v", err)
	}
	if strike.doAttackCalls != 2 {
		t.Fatalf("DoAttack calls after the latch cleared = %d, want 2", strike.doAttackCalls)
	}
	if got := a.CurrentIntention(); got != IntentionIdle {
		t.Fatalf("CurrentIntention() = %v, want %v", got, IntentionIdle)
	}
}

func TestAttackableThinkLeavesQueuedMoveToAndLatchForRunAI(t *testing.T) {
	a, _, strike := latchedAttackAI(t)
	a.AddMoveToDesire(location.Location{X: 100, Y: 100}, 50)

	if err := a.Think(); err != nil {
		t.Fatalf("Think() error: %v", err)
	}
	if got := a.CurrentIntention(); got == IntentionMoveTo {
		t.Fatalf("CurrentIntention() after Think = %v, want the queued walk left for RunAI", got)
	}

	if err := a.RunAI(); err != nil {
		t.Fatalf("RunAI() error: %v", err)
	}
	if strike.doAttackCalls != 2 {
		t.Fatalf("DoAttack calls after the latched RunAI = %d, want 2", strike.doAttackCalls)
	}
	if got := a.CurrentIntention(); got != IntentionAttack {
		t.Fatalf("CurrentIntention() after the latched RunAI = %v, want %v", got, IntentionAttack)
	}
}

// ---- Think continues, never selects ----

// TestAttackableThinkFromIdleLeavesQueuedAttackForRunAI pins that Think
// only continues the current intention: an idle actor with a queued attack
// desire takes no step on Think, and the next RunAI takes the attack up.
func TestAttackableThinkFromIdleLeavesQueuedAttackForRunAI(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	owner.known = map[int32]bool{target.ObjectID(): true}
	strike := &recordingAttack{canAttack: true}
	ai := NewAttackable(owner, &recordingMove{}, strike)
	ai.AddDamageHate(target, 0, 20)
	ai.Desires().AddOrUpdate(&Desire{Kind: IntentionAttack, FinalTarget: target, Weight: 20})

	if err := ai.Think(); err != nil {
		t.Fatalf("Think() error: %v", err)
	}
	if got := ai.CurrentIntention(); got != IntentionIdle {
		t.Fatalf("CurrentIntention() after Think = %v, want %v", got, IntentionIdle)
	}
	if strike.doAttackCalls != 0 {
		t.Fatalf("DoAttack calls after Think = %d, want 0", strike.doAttackCalls)
	}
	if !ai.Desires().Has(&Desire{Kind: IntentionAttack, FinalTarget: target}) {
		t.Fatal("attack desire dropped by Think, want it still queued")
	}

	if err := ai.RunAI(); err != nil {
		t.Fatalf("RunAI() error: %v", err)
	}
	if got := ai.CurrentIntention(); got != IntentionAttack {
		t.Fatalf("CurrentIntention() after RunAI = %v, want %v", got, IntentionAttack)
	}
	if strike.target != target {
		t.Fatalf("attacked target after RunAI = %v, want %v", strike.target, target)
	}
}
