package ai

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

func TestAttackableAIWanderReturnHome(t *testing.T) {
	owner := actor(1)
	owner.inTerritory = false
	owner.returnHome = true
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})

	if err := thinkWanderOnce(ai); err != nil {
		t.Fatalf("Think() error: %v", err)
	}

	if owner.returnHomeCalls != 1 {
		t.Fatalf("ReturnHome calls = %d, want 1", owner.returnHomeCalls)
	}
	if got := ai.CurrentIntention(); got != IntentionWander {
		t.Fatalf("CurrentIntention() = %v, want wander while returning home", got)
	}
}

func TestAttackableAIWanderSkipsReturnHomeWhileMoving(t *testing.T) {
	owner := actor(1)
	owner.inTerritory = false
	owner.returnHome = true
	owner.moving = true
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})

	if err := thinkWanderOnce(ai); err != nil {
		t.Fatalf("Think() error: %v", err)
	}

	if owner.returnHomeCalls != 0 {
		t.Fatalf("ReturnHome calls = %d, want 0 while moving", owner.returnHomeCalls)
	}
}

func TestAttackableAIWanderClearsWhenOutsideTerritoryAndNotReturning(t *testing.T) {
	owner := actor(1)
	owner.inTerritory = false
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})

	if err := thinkWanderOnce(ai); err != nil {
		t.Fatalf("Think() error: %v", err)
	}

	if got := ai.CurrentIntention(); got != IntentionWander {
		t.Fatalf("CurrentIntention() = %v, want wander kept current outside territory without return home", got)
	}
	if ai.Desires().Has(&Desire{Kind: IntentionWander}) {
		t.Fatal("wander desire still queued after out-of-territory thinkWander, want it dropped")
	}
	if got := ai.Desires().Len(); got != 0 {
		t.Fatalf("queued desires = %d, want 0 after out-of-territory wander clear", got)
	}
}

func TestAttackableAIWanderWalksFromSpawnOnFirstStep(t *testing.T) {
	owner := actor(1)
	owner.moveSpeed = 50
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})

	if err := thinkWanderOnce(ai); err != nil {
		t.Fatalf("Think() error: %v", err)
	}

	if owner.walkStanceCalls != 1 {
		t.Fatalf("walk stance calls = %d, want 1", owner.walkStanceCalls)
	}
	if owner.wanderCalls != 1 {
		t.Fatalf("wander move calls = %d, want 1", owner.wanderCalls)
	}
	if owner.wanderOffset != 150 {
		t.Fatalf("wander offset = %d, want 150 (walk speed * 3)", owner.wanderOffset)
	}
}

// TestAttackableAIWanderChainHitWalksAndEnds pins AttackableAI.thinkWander's
// task chain: a WANDER promoted after a wander arms one firing a wander
// timer out and takes no step, periodic cycles leave the current wander
// alone, and a hit walks and schedules nothing more.
func TestAttackableAIWanderChainHitWalksAndEnds(t *testing.T) {
	brain, owner, _, _ := wanderArrivedThenPromoted(t, time.Unix(1_000, 0))

	for range 3 {
		if err := brain.TickThink(); err != nil {
			t.Fatalf("TickThink() error: %v", err)
		}
	}
	if owner.wanderCalls != 1 || len(owner.timers) != 1 {
		t.Fatalf("wander walks/timers after cycles on the current wander = %d/%d, want 1/1", owner.wanderCalls, len(owner.timers))
	}

	owner.fireTimer(t, defaultWanderTimer*time.Second)
	if owner.wanderCalls != 2 {
		t.Fatalf("wander walks after a hit = %d, want 2", owner.wanderCalls)
	}
	if got := len(owner.pendingTimers()); got != 0 {
		t.Fatalf("pending timers after a hit = %d, want 0 (chain ended)", got)
	}
}

// TestAttackableAIWanderChainMissReschedules pins the chain's miss: no walk,
// and the next firing one wander timer later.
func TestAttackableAIWanderChainMissReschedules(t *testing.T) {
	brain, owner, _, _ := wanderArrivedThenPromoted(t, time.Unix(1_000, 0))
	brain.SetRandomWalkRate(0)

	for range 3 {
		owner.fireTimer(t, defaultWanderTimer*time.Second)
		if owner.wanderCalls != 1 {
			t.Fatalf("wander walks with rate 0 = %d, want 1 (the first step only)", owner.wanderCalls)
		}
	}
	if got := len(owner.timers); got != 4 {
		t.Fatalf("timers armed = %d, want 4 (one per miss after the first)", got)
	}
}

// TestAttackableAIWanderChainHitWhileOutOfControlEndsIt pins the chain
// firing while the actor cannot act: the hit's walk is the actor's to
// refuse, the chain ends, and once control returns desire selection does
// not restart the wander that is still current with its desire queued.
func TestAttackableAIWanderChainHitWhileOutOfControlEndsIt(t *testing.T) {
	brain, owner, _, _ := wanderArrivedThenPromoted(t, time.Unix(1_000, 0))
	owner.denyAction = true

	owner.fireTimer(t, defaultWanderTimer*time.Second)
	if owner.wanderCalls != 2 {
		t.Fatalf("wander walk attempts after the out-of-control hit = %d, want 2", owner.wanderCalls)
	}

	owner.denyAction = false
	for range 4 {
		if err := brain.TickThink(); err != nil {
			t.Fatalf("TickThink() error: %v", err)
		}
		if err := brain.RunAI(); err != nil {
			t.Fatalf("RunAI() error: %v", err)
		}
	}
	if got := brain.CurrentIntention(); got != IntentionWander {
		t.Fatalf("CurrentIntention() after control returned = %v, want %v kept", got, IntentionWander)
	}
	if owner.wanderCalls != 2 || len(owner.pendingTimers()) != 0 {
		t.Fatalf("wander walks/pending timers after control returned = %d/%d, want 2/0", owner.wanderCalls, len(owner.pendingTimers()))
	}
}

// TestAttackableAIWanderChainEndsOnDeath pins a dead actor's chain: the
// firing neither walks nor reschedules.
func TestAttackableAIWanderChainEndsOnDeath(t *testing.T) {
	brain, owner, _, _ := wanderArrivedThenPromoted(t, time.Unix(1_000, 0))
	brain.SetRandomWalkRate(0)
	owner.alikeDead = true

	owner.fireTimer(t, defaultWanderTimer*time.Second)
	if owner.wanderCalls != 1 || len(owner.pendingTimers()) != 0 {
		t.Fatalf("wander walks/pending timers after a dead firing = %d/%d, want 1/0", owner.wanderCalls, len(owner.pendingTimers()))
	}
}

func TestAttackableAIAttackInterruptsWander(t *testing.T) {
	owner := actor(1)
	owner.moveSpeed = 50
	target := actor(2)
	owner.known = map[int32]bool{target.ObjectID(): true}
	strike := &recordingAttack{canAttack: true}
	ai := NewAttackable(owner, &recordingMove{}, strike)

	if err := thinkWanderOnce(ai); err != nil {
		t.Fatalf("wander RunAI() error: %v", err)
	}

	addAttackHate(ai, target, 0, 10)
	if err := ai.RunAI(); err != nil {
		t.Fatalf("attack RunAI() error: %v", err)
	}
	if got := ai.CurrentIntention(); got != IntentionAttack {
		t.Fatalf("CurrentIntention() = %v, want attack interrupting wander", got)
	}
	if strike.target != target {
		t.Fatalf("attacked target = %v, want %v", strike.target, target)
	}
}

// wanderArrivedThenPromoted walks one wander step at clock start, arrives,
// and has a THINK (a control effect ending), which takes no wander step.
// The next cycles run at start+1s (lifetime), start+2s (empty-queue idle)
// and start+3s (WANDER re-promoted, arming the wander chain). It returns
// the brain, its actor, the promotion time and a setter for the AI clock.
func wanderArrivedThenPromoted(t *testing.T, start time.Time) (*Attackable, *fakeActor, time.Time, func(time.Time)) {
	t.Helper()
	owner := actor(1)
	owner.idleWander = true
	brain := NewAttackable(owner, &recordingMove{}, &recordingAttack{})
	brain.SetRandomWalkRate(100)
	brain.roll = func(int) int { return 0 }
	now := start
	brain.now = func() time.Time { return now }
	if err := thinkWanderOnce(brain); err != nil {
		t.Fatalf("wander RunAI() error: %v", err)
	}
	if owner.wanderCalls != 1 {
		t.Fatalf("wander walks after the first wander step = %d, want 1", owner.wanderCalls)
	}
	if len(owner.timers) != 0 {
		t.Fatalf("timers after the first wander step = %d, want 0 (it walks at once)", len(owner.timers))
	}
	brain.Arrived()
	if got := brain.CurrentIntention(); got != IntentionWander {
		t.Fatalf("CurrentIntention() after Arrived = %v, want %v kept", got, IntentionWander)
	}
	stances := owner.walkStanceCalls
	if err := brain.Think(); err != nil {
		t.Fatalf("arrival Think() error: %v", err)
	}
	if got := brain.CurrentIntention(); got != IntentionWander {
		t.Fatalf("CurrentIntention() after arrival Think = %v, want %v kept", got, IntentionWander)
	}
	if owner.walkStanceCalls != stances || len(owner.timers) != 0 {
		t.Fatalf("walk stances/timers after arrival Think = %d/%d, want %d/0 (no wander step)", owner.walkStanceCalls, len(owner.timers), stances)
	}

	for i, at := range []int{1, 2, 3} {
		now = start.Add(time.Duration(at) * time.Second)
		if err := brain.TickThink(); err != nil {
			t.Fatalf("TickThink() %d error: %v", i, err)
		}
		if at == 2 {
			if got := brain.CurrentIntention(); got != IntentionIdle {
				t.Fatalf("CurrentIntention() after the empty-queue cycle = %v, want %v", got, IntentionIdle)
			}
		}
	}
	promoted := now
	if got := brain.CurrentIntention(); got != IntentionWander {
		t.Fatalf("CurrentIntention() after the promotion = %v, want %v", got, IntentionWander)
	}
	if pending := owner.pendingTimers(); len(pending) != 1 || pending[0].delay != defaultWanderTimer*time.Second {
		t.Fatalf("pending timers after the promotion = %d, want one wander timer", len(pending))
	}
	if owner.wanderCalls != 1 {
		t.Fatalf("wander walks after the promotion = %d, want 1", owner.wanderCalls)
	}
	return brain, owner, promoted, func(at time.Time) { now = at }
}

// TestAttackableWanderTimerOutlivesIdle pins AttackableAI's pending
// _wanderTask across an idle: when a walk not taken by the wander (the
// hostile's wander recheck step) arrives while the chain runs, the idle and
// the next WANDER promotion keep that firing, since the running task fires
// first and cancels the one the promotion schedules.
func TestAttackableWanderTimerOutlivesIdle(t *testing.T) {
	start := time.Unix(1_000, 0)
	brain, owner, promoted, setNow := wanderArrivedThenPromoted(t, start)
	running := owner.pendingTimers()[0]

	setNow(promoted.Add(time.Second))
	brain.Arrived()
	setNow(promoted.Add(2 * time.Second))
	if err := brain.TickThink(); err != nil {
		t.Fatalf("idle TickThink() error: %v", err)
	}
	if got := brain.CurrentIntention(); got != IntentionIdle {
		t.Fatalf("CurrentIntention() after the empty-queue cycle = %v, want %v", got, IntentionIdle)
	}
	setNow(promoted.Add(3 * time.Second))
	if err := brain.TickThink(); err != nil {
		t.Fatalf("promote TickThink() error: %v", err)
	}
	if got := brain.CurrentIntention(); got != IntentionWander {
		t.Fatalf("CurrentIntention() after the re-promotion = %v, want %v", got, IntentionWander)
	}
	if pending := owner.pendingTimers(); len(pending) != 1 || pending[0] != running {
		t.Fatalf("pending timers after the re-promotion = %d, want only the running one", len(pending))
	}

	owner.fireTimer(t, defaultWanderTimer*time.Second)
	if owner.wanderCalls != 2 {
		t.Fatalf("wander walks when the running timer ran out = %d, want 2", owner.wanderCalls)
	}
}

// TestAttackableWanderFirstStepAfterOtherDesireCancelsChain pins
// AttackableAI.thinkWander keeping one chain per actor: a WANDER promoted
// after a desire of another kind cancels the pending firing before it
// walks at once, and that firing, if it already left its timer, neither
// walks nor schedules.
func TestAttackableWanderFirstStepAfterOtherDesireCancelsChain(t *testing.T) {
	brain, owner, _, _ := wanderArrivedThenPromoted(t, time.Unix(1_000, 0))
	running := owner.pendingTimers()[0]

	brain.Desires().AddOrUpdate(&Desire{Kind: IntentionMoveTo, Location: location.Location{X: 100, Y: 100}, Weight: 1_000})
	if err := brain.RunAI(); err != nil {
		t.Fatalf("move-to RunAI() error: %v", err)
	}
	if got := brain.CurrentIntention(); got != IntentionMoveTo {
		t.Fatalf("CurrentIntention() after the move-to promotion = %v, want %v", got, IntentionMoveTo)
	}
	brain.Arrived()

	if err := thinkWanderOnce(brain); err != nil {
		t.Fatalf("wander RunAI() error: %v", err)
	}
	if got := brain.CurrentIntention(); got != IntentionWander {
		t.Fatalf("CurrentIntention() after the wander re-promotion = %v, want %v", got, IntentionWander)
	}
	if !running.stopped {
		t.Fatal("pending wander firing not stopped by the first step after a move-to")
	}
	if owner.wanderCalls != 2 || len(owner.pendingTimers()) != 0 {
		t.Fatalf("wander walks/pending timers after the first step = %d/%d, want 2/0 (walks at once, no chain)", owner.wanderCalls, len(owner.pendingTimers()))
	}

	timers := len(owner.timers)
	running.fn()
	if owner.wanderCalls != 2 || len(owner.timers) != timers {
		t.Fatalf("wander walks/timers after the cancelled firing ran = %d/%d, want 2/%d", owner.wanderCalls, len(owner.timers), timers)
	}
}

// TestAttackableWanderBackToPeaceStopsChainAndDropsStaleFiring pins the
// chain ending on a return to peace, and a firing of that ended chain,
// run after a new WANDER promotion armed the next one, neither walking
// nor touching the new firing.
func TestAttackableWanderBackToPeaceStopsChainAndDropsStaleFiring(t *testing.T) {
	brain, owner, _, _ := wanderArrivedThenPromoted(t, time.Unix(1_000, 0))
	old := owner.pendingTimers()[0]

	brain.SetBackToPeace()
	if got := len(owner.pendingTimers()); got != 0 {
		t.Fatalf("pending timers after back to peace = %d, want 0 (chain stopped)", got)
	}

	if err := thinkWanderOnce(brain); err != nil {
		t.Fatalf("wander RunAI() error: %v", err)
	}
	pending := owner.pendingTimers()
	if len(pending) != 1 || pending[0] == old {
		t.Fatalf("pending timers after the wander promotion = %d, want one fresh timer", len(pending))
	}
	fresh := pending[0]

	timers := len(owner.timers)
	old.fn()
	if owner.wanderCalls != 1 || len(owner.timers) != timers {
		t.Fatalf("wander walks/timers after the stale firing = %d/%d, want 1/%d", owner.wanderCalls, len(owner.timers), timers)
	}
	if pending := owner.pendingTimers(); len(pending) != 1 || pending[0] != fresh {
		t.Fatal("stale firing disturbed the fresh chain")
	}
	owner.fireTimer(t, defaultWanderTimer*time.Second)
	if owner.wanderCalls != 2 {
		t.Fatalf("wander walks after the fresh firing = %d, want 2", owner.wanderCalls)
	}
}

// TestAttackableWanderTimerEndsWhileNotWandering pins that a wander firing
// while the actor is idle finds no WANDER and ends: the next wander
// promotion arms a fresh timer instead of walking at once.
func TestAttackableWanderTimerEndsWhileNotWandering(t *testing.T) {
	start := time.Unix(1_000, 0)
	brain, owner, promoted, setNow := wanderArrivedThenPromoted(t, start)
	stale := owner.pendingTimers()[0]

	setNow(promoted.Add(time.Second))
	brain.Arrived()
	setNow(promoted.Add(2 * time.Second))
	if err := brain.TickThink(); err != nil {
		t.Fatalf("idle TickThink() error: %v", err)
	}
	if got := brain.CurrentIntention(); got != IntentionIdle {
		t.Fatalf("CurrentIntention() after the empty-queue cycle = %v, want %v", got, IntentionIdle)
	}
	owner.fireTimer(t, defaultWanderTimer*time.Second)
	if owner.wanderCalls != 1 || len(owner.pendingTimers()) != 0 {
		t.Fatalf("wander walks/pending timers after an idle firing = %d/%d, want 1/0", owner.wanderCalls, len(owner.pendingTimers()))
	}

	if err := brain.TickThink(); err != nil {
		t.Fatalf("promote TickThink() error: %v", err)
	}
	if got := brain.CurrentIntention(); got != IntentionWander {
		t.Fatalf("CurrentIntention() after the re-promotion = %v, want %v", got, IntentionWander)
	}
	if owner.wanderCalls != 1 {
		t.Fatalf("wander walks after the re-promotion = %d, want 1 (stale timer must not roll)", owner.wanderCalls)
	}
	if pending := owner.pendingTimers(); len(pending) != 1 || pending[0] == stale {
		t.Fatalf("pending timers after the re-promotion = %d, want one fresh timer", len(pending))
	}
}

// TestAttackableThinkDuringWanderTakesNoStep pins that the THINK event has
// no wander step and does not swap a wander for a queued attack: the walk
// and the attack wait for desire selection.
func TestAttackableThinkDuringWanderTakesNoStep(t *testing.T) {
	owner := actor(1)
	owner.moveSpeed = 50
	target := actor(2)
	owner.known = map[int32]bool{target.ObjectID(): true}
	strike := &recordingAttack{canAttack: true}
	ai := NewAttackable(owner, &recordingMove{}, strike)
	ai.SetRandomWalkRate(100)
	ai.roll = func(int) int { return 0 }
	if err := thinkWanderOnce(ai); err != nil {
		t.Fatalf("wander RunAI() error: %v", err)
	}
	if got := ai.CurrentIntention(); got != IntentionWander {
		t.Fatalf("CurrentIntention() = %v, want %v", got, IntentionWander)
	}
	walks, stances := owner.wanderCalls, owner.walkStanceCalls

	ai.AddDamageHate(target, 0, 20)
	ai.Desires().AddOrUpdate(&Desire{Kind: IntentionAttack, FinalTarget: target, Weight: 20})
	if err := ai.Think(); err != nil {
		t.Fatalf("Think() error: %v", err)
	}
	if got := ai.CurrentIntention(); got != IntentionWander {
		t.Fatalf("CurrentIntention() after Think = %v, want %v kept", got, IntentionWander)
	}
	if owner.wanderCalls != walks || owner.walkStanceCalls != stances {
		t.Fatalf("wander walks/stances after Think = %d/%d, want %d/%d (no wander step)", owner.wanderCalls, owner.walkStanceCalls, walks, stances)
	}
	if strike.doAttackCalls != 0 {
		t.Fatalf("DoAttack calls after Think = %d, want 0", strike.doAttackCalls)
	}

	if err := ai.RunAI(); err != nil {
		t.Fatalf("RunAI() error: %v", err)
	}
	if got := ai.CurrentIntention(); got != IntentionAttack {
		t.Fatalf("CurrentIntention() after RunAI = %v, want %v", got, IntentionAttack)
	}
}
