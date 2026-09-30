package ai

import (
	"bytes"
	"math"
	"slices"
	"sync"
	"testing"
	"time"

	modelactor "github.com/fatal10110/acis_golang/internal/gameserver/model/actor"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable/attackabletest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/rs/zerolog"
)

// ---- from attackable_attack_test.go ----
// addAttackHate is test scaffolding for seeding multiple attackers' hate and
// queued attack Desire before a test's own explicit Think()/TickThink()
// call. It writes the threat table directly (AddDamageHate) and queues the
// Desire directly, bypassing the public AddAttackDesire so bulk setup never
// triggers that method's own "first reaction runs immediately" side effect
// (see thinkIfNoMostHated and TestAttackableAIAddAttackDesireFeedsThreatTable
// for that behavior in isolation).
func addAttackHate(ai *Attackable, attacker attackable.Combatant, damage, hate float64) {
	ai.AddDamageHate(attacker, damage, hate)
	ai.Desires().AddOrUpdate(&Desire{
		Kind:         IntentionAttack,
		FinalTarget:  attacker,
		Weight:       hate,
		QueuedAt:     time.Now(),
		MoveToTarget: true,
	})
}

func tickThinkIdle(ai *Attackable) error {
	if err := ai.TickThink(); err != nil {
		return err
	}
	return ai.TickThink()
}

func tickThinkPromote(ai *Attackable) error {
	if err := tickThinkIdle(ai); err != nil {
		return err
	}
	return ai.TickThink()
}

func thinkWanderOnce(ai *Attackable) error {
	ai.Desires().AddOrUpdate(&Desire{Kind: IntentionWander, Timer: 5, Weight: 5})
	return ai.RunAI()
}

func TestAttackableAIAddDamageHateDoesNotQueueAttackDesire(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})

	ai.AddDamageHate(target, 7, 30)

	if got := ai.Threats().Hate(target); got != 30 {
		t.Fatalf("hate = %v, want 30", got)
	}
	if got := ai.Desires().Len(); got != 0 {
		t.Fatalf("queued desires = %d, want 0", got)
	}
}

// TestAttackableAIAddAttackDesireFeedsThreatTable pins #2340: Java's
// NpcAI.addAttackDesire is the single choke point that both queues the
// Desire and, under its updateAggro=true default, writes the same weight
// into the AggroList/threat table (NpcAI.java:713-727). Every Go call site
// currently uses that default, so AddAttackDesire alone must raise the
// target's threat hate, not just queue a Desire.
func TestAttackableAIAddAttackDesireFeedsThreatTable(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})

	ai.AddAttackDesire(target, 200)

	if got := ai.Threats().Hate(target); got != 200 {
		t.Fatalf("hate = %v, want 200", got)
	}
	if threat, ok := ai.Threats().Get(target); !ok || threat.Damage != 0 {
		t.Fatalf("threat = (%+v, %v), want damage 0 (weight-only convenience call)", threat, ok)
	}
}

// TestAttackableAICombatDamageHateUsesCallerWeightNotDamage pins #2340:
// Npc.reduceCurrentHp's own addDamageHate(attacker, damage, 0) call never
// raises hate (Npc.java:395); real hate comes only from the ATTACKED-event
// attack Desire queued alongside it, at whatever weight the caller derived
// (the per-script onAttacked formula — see Hostile.attackedHateWeight, which
// this generic AI layer does not know about). AddCombatDamageHate's
// resulting threat hate must equal that caller-supplied weight, not the raw
// damage passed for the threat table's damage bookkeeping.
func TestAttackableAICombatDamageHateUsesCallerWeightNotDamage(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})

	ai.AddCombatDamageHate(target, 9999, 42)

	if got := ai.Threats().Hate(target); got != 42 {
		t.Fatalf("hate = %v, want caller-supplied weight 42, not raw damage", got)
	}
	if threat, ok := ai.Threats().Get(target); !ok || threat.Damage != 9999 {
		t.Fatalf("threat = (%+v, %v), want damage 9999 preserved", threat, ok)
	}
}

func TestAttackableAIChoosesMostHatedTargetToAttack(t *testing.T) {
	owner := actor(1)
	low := actor(2)
	high := actor(3)
	owner.known = map[int32]bool{low.ObjectID(): true, high.ObjectID(): true}
	owner.attackRange = 40
	move := &recordingMove{}
	strike := &recordingAttack{canAttack: true}
	ai := NewAttackable(owner, move, strike)

	addAttackHate(ai, low, 0, 10)
	addAttackHate(ai, high, 0, 25)
	ai.RunAI()

	if got := ai.CurrentIntention(); got != IntentionAttack {
		t.Fatalf("CurrentIntention() = %v, want %v", got, IntentionAttack)
	}
	if strike.target != high {
		t.Fatalf("attacked target = %v, want high threat target", strike.target)
	}
	if move.stopCount != 1 {
		t.Fatalf("stop count = %d, want 1", move.stopCount)
	}
	if move.followTarget != high || move.followRange != 40 {
		t.Fatalf("follow check = (%v, %d), want (%v, 40)", move.followTarget, move.followRange, high)
	}
}

// TestAttackableRunAIStopsMovementAndAttacksOnTheSameTick pins thinkAttack's
// two-call shape: an accepted swing cancels the walk and starts the attack in
// the same tick, and reports no error for either.
func TestAttackableRunAIStopsMovementAndAttacksOnTheSameTick(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	owner.known = map[int32]bool{target.ObjectID(): true}
	move := &recordingMove{}
	strike := &recordingAttack{canAttack: true}
	ai := NewAttackable(owner, move, strike)

	addAttackHate(ai, target, 0, 10)
	if err := ai.RunAI(); err != nil {
		t.Fatalf("RunAI() error = %v, want nil", err)
	}

	if move.stopCount != 1 {
		t.Fatalf("move.Stop calls = %d, want 1", move.stopCount)
	}
	if strike.doAttackCalls != 1 || strike.target != target {
		t.Fatalf("DoAttack calls = (%d, %v), want (1, target)", strike.doAttackCalls, strike.target)
	}
}

func TestAttackableAIStartsOffensiveFollowBeforeAttack(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	owner.known = map[int32]bool{target.ObjectID(): true}
	owner.attackRange = 80
	move := &recordingMove{followStarted: true}
	strike := &recordingAttack{canAttack: true}
	ai := NewAttackable(owner, move, strike)

	addAttackHate(ai, target, 0, 100)
	ai.RunAI()

	if move.followTarget != target || move.followRange != 80 {
		t.Fatalf("follow check = (%v, %d), want (%v, 80)", move.followTarget, move.followRange, target)
	}
	if strike.target != nil {
		t.Fatalf("attacked target = %v, want none while follow starts", strike.target)
	}
	if move.stopCount != 0 {
		t.Fatalf("stop count = %d, want 0 while follow starts", move.stopCount)
	}
}

func TestAttackableAIQueuesAttackWhileBusy(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	owner.known = map[int32]bool{target.ObjectID(): true}
	move := &recordingMove{}
	strike := &recordingAttack{canAttack: true, attackingNow: true}
	ai := NewAttackable(owner, move, strike)

	addAttackHate(ai, target, 0, 100)
	ai.RunAI()

	next, nextTarget, ok := ai.NextIntention()
	if !ok {
		t.Fatal("NextIntention() ok = false, want true")
	}
	if next != IntentionAttack || nextTarget != target {
		t.Fatalf("NextIntention() = (%v, %v), want (%v, target)", next, nextTarget, IntentionAttack)
	}
	if strike.target != nil {
		t.Fatalf("attacked target = %v, want none while already attacking", strike.target)
	}
}

func TestAttackableAIIgnoresLostTarget(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	owner.known = map[int32]bool{target.ObjectID(): false}
	move := &recordingMove{}
	strike := &recordingAttack{canAttack: true}
	ai := NewAttackable(owner, move, strike)

	addAttackHate(ai, target, 0, 100)
	ai.RunAI()

	if move.followTarget != nil {
		t.Fatalf("follow target = %v, want none for lost target", move.followTarget)
	}
	if strike.target != nil {
		t.Fatalf("attacked target = %v, want none for lost target", strike.target)
	}
}

// TestAttackableAIKeepsTargetWhenCanAttackFails is the regression test for
// PR #936's skipAttackTarget: on a CanAttack failure the reference
// (CreatureAI.thinkAttack, `if (!_actor.getAttack().canAttack(target)) return;`)
// leaves the current target, its hate and its ATTACK desire untouched and
// retries next tick. The removed skipAttackTarget instead zeroed the
// blocked target's hate, dropped its desire, and transferred the hate to
// the next most-hated attacker with no validity filter.
func TestAttackableAIKeepsTargetWhenCanAttackFails(t *testing.T) {
	owner := actor(1)
	blocked := actor(2)
	other := actor(3)
	owner.known = map[int32]bool{blocked.ObjectID(): true, other.ObjectID(): true}
	move := &recordingMove{}
	strike := &recordingAttack{
		canAttackTarget: map[int32]bool{
			blocked.ObjectID(): false,
			other.ObjectID():   true,
		},
	}
	ai := NewAttackable(owner, move, strike)

	addAttackHate(ai, other, 0, 25)
	addAttackHate(ai, blocked, 0, 100)
	ai.RunAI()

	if strike.target != nil {
		t.Fatalf("attacked target = %v, want none while blocked target is retried", strike.target)
	}
	if got := ai.CurrentIntention(); got != IntentionAttack {
		t.Fatalf("CurrentIntention() = %v, want %v (kept committed to blocked target)", got, IntentionAttack)
	}
	if got := ai.Threats().Hate(blocked); got != 100 {
		t.Fatalf("blocked target hate = %v, want untouched 100", got)
	}
	if got := ai.Threats().Hate(other); got != 25 {
		t.Fatalf("other target hate = %v, want untouched 25 (no hate transfer)", got)
	}
}

// ---- from attackable_cast_test.go ----
func TestAttackableAIPromotesQueuedCastDesireAndCasts(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	owner.known = map[int32]bool{target.ObjectID(): true}
	move := &recordingMove{}
	ref := skill.Ref{ID: 4, Level: 1}
	cast := &recordingCast{canAttempt: true, canCast: true, castRange: 400}
	ai := NewAttackable(owner, move, &recordingAttack{})
	ai.SetCastController(cast)

	ai.Desires().AddOrUpdate(&Desire{Kind: IntentionCast, FinalTarget: target, Skill: ref, Weight: 10})
	ai.RunAI()

	if got := ai.CurrentIntention(); got != IntentionCast {
		t.Fatalf("CurrentIntention() = %v, want %v", got, IntentionCast)
	}
	if !cast.castCalled || cast.castedTarget != target || cast.castedRef != ref {
		t.Fatalf("Cast call = (%v, %v, %v), want (true, target, %v)", cast.castCalled, cast.castedTarget, cast.castedRef, ref)
	}
	if move.followTarget != target || move.followRange != 400 {
		t.Fatalf("follow check = (%v, %d), want (%v, 400)", move.followTarget, move.followRange, target)
	}
}

func TestAttackableAICastStopsMovementAndFacesTargetForLongCast(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	owner.known = map[int32]bool{target.ObjectID(): true}
	move := &recordingMove{}
	ref := skill.Ref{ID: 4, Level: 1}
	cast := &recordingCast{canAttempt: true, canCast: true, stopsMove: true}
	ai := NewAttackable(owner, move, &recordingAttack{})
	ai.SetCastController(cast)

	ai.Desires().AddOrUpdate(&Desire{Kind: IntentionCast, FinalTarget: target, Skill: ref, Weight: 10})
	ai.RunAI()

	if move.stopCount != 1 {
		t.Fatalf("stop count = %d, want 1", move.stopCount)
	}
	if owner.headingTarget != target {
		t.Fatalf("heading target = %v, want target", owner.headingTarget)
	}
	if !cast.castCalled {
		t.Fatal("Cast() not called for a long-hit-time skill")
	}
}

func TestAttackableAICastDoesNotFaceSelfTarget(t *testing.T) {
	owner := actor(1)
	// A creature's own region always contains itself, so it always "knows"
	// itself; the fake's known map mirrors that explicitly here since it
	// otherwise only tracks other actors.
	owner.known[owner.ObjectID()] = true
	move := &recordingMove{}
	ref := skill.Ref{ID: 4, Level: 1}
	cast := &recordingCast{canAttempt: true, canCast: true, stopsMove: true}
	ai := NewAttackable(owner, move, &recordingAttack{})
	ai.SetCastController(cast)

	ai.Desires().AddOrUpdate(&Desire{Kind: IntentionCast, FinalTarget: owner, Skill: ref, Weight: 10})
	ai.RunAI()

	if owner.headingTarget != nil {
		t.Fatalf("heading target = %v, want none for self-targeted skill", owner.headingTarget)
	}
	if !cast.castCalled {
		t.Fatal("Cast() not called for a self-targeted skill")
	}
}

func TestAttackableAICastStartsOffensiveFollowBeforeCasting(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	owner.known = map[int32]bool{target.ObjectID(): true}
	move := &recordingMove{followStarted: true}
	ref := skill.Ref{ID: 4, Level: 1}
	cast := &recordingCast{canAttempt: true, canCast: true, castRange: 400}
	ai := NewAttackable(owner, move, &recordingAttack{})
	ai.SetCastController(cast)

	ai.Desires().AddOrUpdate(&Desire{Kind: IntentionCast, FinalTarget: target, Skill: ref, Weight: 10})
	ai.RunAI()

	if cast.castCalled {
		t.Fatal("Cast() called while still closing distance")
	}
	if move.followTarget != target || move.followRange != 400 {
		t.Fatalf("follow check = (%v, %d), want (%v, 400)", move.followTarget, move.followRange, target)
	}
	if owner.runStanceCalls != 1 {
		t.Fatalf("run stance calls = %d, want 1 (cast-approach follow switches to run)", owner.runStanceCalls)
	}
}

func TestAttackableAICastRespectsPreMovementCooldownGate(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	owner.known = map[int32]bool{target.ObjectID(): true}
	move := &recordingMove{}
	ref := skill.Ref{ID: 4, Level: 1}
	cast := &recordingCast{canAttempt: false, canCast: true}
	ai := NewAttackable(owner, move, &recordingAttack{})
	ai.SetCastController(cast)

	ai.Desires().AddOrUpdate(&Desire{Kind: IntentionCast, FinalTarget: target, Skill: ref, Weight: 10})
	ai.RunAI()

	if move.followTarget != nil {
		t.Fatalf("follow target = %v, want none while skill is on cooldown", move.followTarget)
	}
	if cast.castCalled {
		t.Fatal("Cast() called while skill is on cooldown")
	}
}

func TestAttackableAICastRespectsFinalCastGate(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	owner.known = map[int32]bool{target.ObjectID(): true}
	move := &recordingMove{}
	ref := skill.Ref{ID: 4, Level: 1}
	cast := &recordingCast{canAttempt: true, canCast: false}
	ai := NewAttackable(owner, move, &recordingAttack{})
	ai.SetCastController(cast)

	ai.Desires().AddOrUpdate(&Desire{Kind: IntentionCast, FinalTarget: target, Skill: ref, Weight: 10})
	ai.RunAI()

	if cast.castCalled {
		t.Fatal("Cast() called after the final cast gate rejected the attempt")
	}
	if owner.moveToPawnCalls != 1 || owner.moveToPawnTo != target {
		t.Fatalf("BroadcastMoveToPawn calls = (%d, %v), want (1, target)", owner.moveToPawnCalls, owner.moveToPawnTo)
	}
}

// TestAttackableRunAICastStopsMovementAndStillFacesTarget covers the
// rejected-cast path: a cast whose skill freezes the caster cancels the walk,
// and the rotation-only notice observers need still goes out on the same tick
// even though the cast itself was rejected.
func TestAttackableRunAICastStopsMovementAndStillFacesTarget(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	owner.known = map[int32]bool{target.ObjectID(): true}
	move := &recordingMove{}
	ref := skill.Ref{ID: 4, Level: 1}
	cast := &recordingCast{canAttempt: true, canCast: false, stopsMove: true}
	ai := NewAttackable(owner, move, &recordingAttack{})
	ai.SetCastController(cast)

	ai.Desires().AddOrUpdate(&Desire{Kind: IntentionCast, FinalTarget: target, Skill: ref, Weight: 10})
	if err := ai.RunAI(); err != nil {
		t.Fatalf("RunAI() error = %v, want nil", err)
	}

	if move.stopCount != 1 {
		t.Fatalf("move.Stop calls = %d, want 1", move.stopCount)
	}
	if owner.moveToPawnCalls != 1 || owner.moveToPawnTo != target {
		t.Fatalf("BroadcastMoveToPawn calls = (%d, %v), want (1, target)", owner.moveToPawnCalls, owner.moveToPawnTo)
	}
}

func TestAttackableAICastFinalGateRejectDoesNotBroadcastForSelfTarget(t *testing.T) {
	owner := actor(1)
	owner.known[owner.ObjectID()] = true
	move := &recordingMove{}
	ref := skill.Ref{ID: 4, Level: 1}
	cast := &recordingCast{canAttempt: true, canCast: false}
	ai := NewAttackable(owner, move, &recordingAttack{})
	ai.SetCastController(cast)

	ai.Desires().AddOrUpdate(&Desire{Kind: IntentionCast, FinalTarget: owner, Skill: ref, Weight: 10})
	ai.RunAI()

	if owner.moveToPawnCalls != 0 {
		t.Fatalf("BroadcastMoveToPawn calls = %d, want 0 for a self-targeted skill", owner.moveToPawnCalls)
	}
}

func TestAttackableAICastSummonFriendBypassesTargetLostCheck(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	owner.known = map[int32]bool{target.ObjectID(): false}
	move := &recordingMove{}
	ref := skill.Ref{ID: 4, Level: 1}
	cast := &recordingCast{canAttempt: true, canCast: true, skillType: "SUMMON_FRIEND"}
	ai := NewAttackable(owner, move, &recordingAttack{})
	ai.SetCastController(cast)

	ai.Desires().AddOrUpdate(&Desire{Kind: IntentionCast, FinalTarget: target, Skill: ref, Weight: 10})
	ai.RunAI()

	if !cast.castCalled || cast.castedTarget != target {
		t.Fatal("Cast() not called for a SUMMON_FRIEND cast against an unknown target")
	}
}

func TestAttackableAIIgnoresCastDesireForLostTarget(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	owner.known = map[int32]bool{target.ObjectID(): false}
	move := &recordingMove{}
	ref := skill.Ref{ID: 4, Level: 1}
	cast := &recordingCast{canAttempt: true, canCast: true}
	ai := NewAttackable(owner, move, &recordingAttack{})
	ai.SetCastController(cast)

	ai.Desires().AddOrUpdate(&Desire{Kind: IntentionCast, FinalTarget: target, Skill: ref, Weight: 10})
	ai.RunAI()

	if cast.castCalled {
		t.Fatal("Cast() called for a lost target")
	}
}

func TestAttackableAICastNoOpsWithoutCastController(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	owner.known = map[int32]bool{target.ObjectID(): true}
	move := &recordingMove{}
	ref := skill.Ref{ID: 4, Level: 1}
	ai := NewAttackable(owner, move, &recordingAttack{})

	ai.Desires().AddOrUpdate(&Desire{Kind: IntentionCast, FinalTarget: target, Skill: ref, Weight: 10})

	ai.RunAI() // must not panic with no CastController wired.

	if got := ai.CurrentIntention(); got != IntentionCast {
		t.Fatalf("CurrentIntention() = %v, want %v", got, IntentionCast)
	}
}

// ---- from attackable_test.go ----
// fakeActor stands in for AttackableActor. npc.Hostile implements every
// method, but npc imports ai (hostile.go uses ai.Attackable/MoveController),
// so ai's own test package cannot import npc back without an import cycle.
// Kept as-is per docs/agents/test-strategy.md.
type fakeActor struct {
	attackabletest.Combatant
	world.Presence
	id              int32
	siegeGuard      bool
	alikeDead       bool
	denyAction      bool
	attackRange     int
	known           map[int32]bool
	inTerritory     bool
	returnHome      bool
	returnHomeCalls int
	moving          bool
	idleWander      bool
	moveSpeed       float64
	wanderCalls     int
	wanderOffset    int
	walkStanceCalls int
	runStanceCalls  int
	headingRestores int
	x, y, z         int
	headingTarget   attackable.Combatant
	moveToPawnCalls int
	moveToPawnTo    attackable.Combatant
	refusals        int
}

func actor(id int32) *fakeActor {
	return &fakeActor{id: id, attackRange: 40, known: make(map[int32]bool), inTerritory: true}
}

func (a *fakeActor) ObjectID() int32     { return a.id }
func (*fakeActor) Kind() modelactor.Kind { return modelactor.KindNPC }
func (a *fakeActor) SiegeGuard() bool    { return a.siegeGuard }
func (a *fakeActor) AlikeDead() bool     { return a.alikeDead }
func (a *fakeActor) DenyAIAction() bool {
	return a.denyAction
}

func (a *fakeActor) Knows(target attackable.Combatant) bool {
	known, ok := a.known[target.ObjectID()]
	return !ok || known
}
func (a *fakeActor) PhysicalAttackRange() int { return a.attackRange }
func (a *fakeActor) ReturnHome() bool {
	a.returnHomeCalls++
	return a.returnHome
}
func (a *fakeActor) IsMoving() bool            { return a.moving }
func (a *fakeActor) InTerritory() bool         { return a.inTerritory }
func (a *fakeActor) Position() (int, int, int) { return a.x, a.y, a.z }
func (a *fakeActor) SetHeadingTo(target attackable.Combatant) {
	a.headingTarget = target
}

func (a *fakeActor) RefuseAttackTarget() { a.refusals++ }

func (a *fakeActor) BroadcastMoveToPawn(target attackable.Combatant) {
	a.moveToPawnCalls++
	a.moveToPawnTo = target
}
func (a *fakeActor) ShouldIdleWander() bool       { return a.idleWander }
func (a *fakeActor) ForceWalkStance()             { a.walkStanceCalls++ }
func (a *fakeActor) ForceRunStance()              { a.runStanceCalls++ }
func (a *fakeActor) RestoreSpawnHeadingIfAtHome() { a.headingRestores++ }
func (a *fakeActor) RealMoveSpeed() float64       { return a.moveSpeed }
func (a *fakeActor) MoveFromSpawnUsingRandomOffset(offset int) {
	a.wanderCalls++
	a.wanderOffset = offset
}

func (*fakeActor) Now() time.Time { return time.Now() }

// recordingMove/recordingAttack/recordingCast (below) stand in for
// MoveController/AttackController/CastController. move.Controller,
// attack.Controller and cast.AIController each satisfy the respective
// interface with no import cycle, but their internal state (attacking,
// bowCooling, disabled, ...) is only reachable by driving the real
// movement/attack/cast subsystem end-to-end, not by setting a field. These
// tests target Attackable's decision branches, so building the real
// controllers is disproportionate per docs/agents/test-strategy.md. Kept
// as-is.
type recordingMove struct {
	followStarted bool
	followTarget  attackable.Combatant
	followRange   int
	followCalls   int
	stopCount     int
	home          location.Location
	denyMove      bool
	// outOfReach is what HoldOffensiveFollow reports; holdCalls counts it.
	outOfReach bool
	holdCalls  int
}

func (m *recordingMove) MaybeStartOffensiveFollow(target attackable.Combatant, attackRange int) (bool, error) {
	m.followCalls++
	m.followTarget = target
	m.followRange = attackRange
	return m.followStarted, nil
}

func (m *recordingMove) HoldOffensiveFollow(target attackable.Combatant, attackRange int) bool {
	m.holdCalls++
	m.followTarget = target
	m.followRange = attackRange
	return m.outOfReach
}

func (m *recordingMove) MoveHome(home location.Location) error {
	m.home = home
	return nil
}

func (m *recordingMove) CanMoveTo(location.Location) bool { return !m.denyMove }

func (m *recordingMove) Stop() { m.stopCount++ }

func (m *recordingMove) CancelFollow() {}

type recordingAttack struct {
	canAttack       bool
	canAttackTarget map[int32]bool
	attackingNow    bool
	bowCooling      bool
	target          attackable.Combatant
	doAttackCalls   int
	stopCalls       int
}

func (a *recordingAttack) BowCoolingDown() bool { return a.bowCooling }
func (a *recordingAttack) AttackingNow() bool   { return a.attackingNow }
func (a *recordingAttack) CanAttack(target attackable.Combatant) bool {
	if a.canAttackTarget != nil {
		return a.canAttackTarget[target.ObjectID()]
	}
	return a.canAttack
}

func (a *recordingAttack) DoAttack(target attackable.Combatant) {
	a.doAttackCalls++
	a.target = target
}

func (a *recordingAttack) Stop() {
	a.stopCalls++
	a.attackingNow = false
}

type recordingCast struct {
	disabled   bool
	casting    bool
	canAttempt bool
	canCast    bool
	hpMpFail   bool
	stopsMove  bool
	castRange  int
	skillType  string
	// final, when set, is every request's final target; noFinal drops
	// every request as having none.
	final   attackable.Combatant
	noFinal bool

	castCalled   bool
	castCalls    int
	castedTarget attackable.Combatant
	castedRef    skill.Ref
	stopCalls    int
	// onStop, when set, runs inside Stop the way a cast end reported by
	// the controller's sink does.
	onStop func()
}

func (c *recordingCast) Disabled() bool               { return c.disabled }
func (c *recordingCast) CastingNow() bool             { return c.casting }
func (c *recordingCast) Range(ref skill.Ref) int      { return c.castRange }
func (c *recordingCast) StopsMovement(skill.Ref) bool { return c.stopsMove }
func (c *recordingCast) SkillType(skill.Ref) string   { return c.skillType }

func (c *recordingCast) CanAttempt(target attackable.Combatant, ref skill.Ref) bool {
	return c.canAttempt
}

func (c *recordingCast) CanCast(target attackable.Combatant, ref skill.Ref) bool {
	return c.canCast
}

func (c *recordingCast) FinalTarget(target attackable.Combatant, ref skill.Ref) attackable.Combatant {
	switch {
	case c.noFinal:
		return nil
	case c.final != nil:
		return c.final
	}
	return target
}

func (c *recordingCast) AttemptCast(target attackable.Combatant, ref skill.Ref) bool {
	return c.CanAttempt(target, ref)
}

func (c *recordingCast) CanCastPlayable(target attackable.Combatant, ref skill.Ref, ctrl bool) bool {
	return c.CanCast(target, ref)
}

func (c *recordingCast) MeetsHPMPDisabled(target attackable.Combatant, ref skill.Ref) bool {
	return !c.hpMpFail
}

func (c *recordingCast) Cast(target attackable.Combatant, ref skill.Ref) {
	c.castCalled = true
	c.castCalls++
	c.castedTarget = target
	c.castedRef = ref
}

func (c *recordingCast) Stop() {
	c.stopCalls++
	c.casting = false
	if c.onStop != nil {
		c.onStop()
	}
}

// ---- from attackable_threat_test.go ----
func TestAttackableAITickDecaysThreatEveryThirdTick(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})
	addAttackHate(ai, target, 0, 20)

	ai.Tick()
	ai.Tick()
	if got := ai.Threats().Hate(target); got != 20 {
		t.Fatalf("hate after two ticks = %v, want 20", got)
	}

	ai.Tick()
	if got, want := ai.Threats().Hate(target), 13.4; math.Abs(got-want) > 0.000001 {
		t.Fatalf("hate after third tick = %v, want %v", got, want)
	}
}

func TestAttackableAITickDecaysCastAndNothingDesireWeights(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})
	ai.Desires().AddOrUpdate(&Desire{Kind: IntentionCast, FinalTarget: target, Skill: skill.Ref{ID: 4, Level: 1}, Weight: 70000})
	ai.Desires().AddOrUpdate(&Desire{Kind: IntentionNothing, Weight: 1})

	ai.Tick()
	ai.Tick()
	got, ok := ai.Desires().Peek()
	if !ok || got.Kind != IntentionCast || got.Weight != 70000 {
		t.Fatalf("Peek after two ticks = (%v, %v), want CAST 70000", got, ok)
	}

	ai.Tick()
	got, ok = ai.Desires().Peek()
	if !ok || got.Kind != IntentionCast {
		t.Fatalf("Peek after third tick = (%v, %v), want CAST", got, ok)
	}
	if math.Abs(got.Weight-4000) > 0.000001 {
		t.Fatalf("CAST weight after third tick = %v, want 4000", got.Weight)
	}
	ai.Desires().RemoveKind(IntentionCast)
	got, ok = ai.Desires().Peek()
	if !ok || got.Kind != IntentionNothing {
		t.Fatalf("Peek NOTHING after CAST removed = (%v, %v), want NOTHING", got, ok)
	}
	if math.Abs(got.Weight-0.5) > 0.000001 {
		t.Fatalf("NOTHING weight after third tick = %v, want 0.5", got.Weight)
	}
}

func TestAttackableAITickDropsCastDesireBelowDecayAmount(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})
	ai.Desires().AddOrUpdate(&Desire{Kind: IntentionCast, FinalTarget: target, Skill: skill.Ref{ID: 4, Level: 1}, Weight: 50000})

	ai.Tick()
	ai.Tick()
	ai.Tick()

	if got := ai.Desires().Len(); got != 0 {
		t.Fatalf("queued desires after CAST decay = %d, want 0", got)
	}
}

func TestAttackableRunAIPrunesZeroWeightCastDesire(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	owner.known = map[int32]bool{target.ObjectID(): true}
	cast := &recordingCast{canAttempt: true, canCast: true}
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})
	ai.SetCastController(cast)
	ai.Desires().AddOrUpdate(&Desire{Kind: IntentionCast, FinalTarget: target, Skill: skill.Ref{ID: 4, Level: 1}, Weight: 0})

	ai.RunAI()

	if got := ai.Desires().Len(); got != 0 {
		t.Fatalf("queued desires = %d, want 0", got)
	}
	if got := ai.CurrentIntention(); got != IntentionIdle {
		t.Fatalf("CurrentIntention() = %v, want %v", got, IntentionIdle)
	}
	if cast.castCalled {
		t.Fatal("Cast called for zero-weight CAST desire")
	}
}

func TestAttackableRunAIPrunesCastWhenHPMPDisabledFails(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	owner.known = map[int32]bool{target.ObjectID(): true}
	cast := &recordingCast{canAttempt: true, canCast: true, hpMpFail: true}
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})
	ai.SetCastController(cast)
	ai.Desires().AddOrUpdate(&Desire{Kind: IntentionCast, FinalTarget: target, Skill: skill.Ref{ID: 4, Level: 1}, Weight: 100})

	ai.RunAI()

	if got := ai.Desires().Len(); got != 0 {
		t.Fatalf("queued desires = %d, want 0", got)
	}
	if got := ai.CurrentIntention(); got != IntentionIdle {
		t.Fatalf("CurrentIntention() = %v, want %v", got, IntentionIdle)
	}
	if cast.castCalled {
		t.Fatal("Cast called for CAST desire that failed HP/MP/mute")
	}
}

func TestAttackableRunAIPrunesAttackDesireBeyond1500(t *testing.T) {
	owner := actor(1)
	near := actor(2)
	far := actor(3)
	far.z = 1501
	owner.known = map[int32]bool{near.ObjectID(): true, far.ObjectID(): true}
	strike := &recordingAttack{canAttack: true}
	ai := NewAttackable(owner, &recordingMove{}, strike)
	addAttackHate(ai, far, 0, 100)
	addAttackHate(ai, near, 0, 50)

	ai.RunAI()

	if strike.target != near {
		t.Fatalf("attacked target = %v, want nearer attacker (far desire pruned)", strike.target)
	}
	if ai.Desires().Has(&Desire{Kind: IntentionAttack, FinalTarget: far}) {
		t.Fatal("far ATTACK desire still queued")
	}
}

func TestAttackableRunAIKeepsAttackDesireAt1500(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	target.x = 1500
	owner.known = map[int32]bool{target.ObjectID(): true}
	strike := &recordingAttack{canAttack: true}
	ai := NewAttackable(owner, &recordingMove{}, strike)
	addAttackHate(ai, target, 0, 20)

	ai.RunAI()

	if strike.target != target {
		t.Fatalf("attacked target = %v, want target at exactly 1500", strike.target)
	}
}

func TestAttackableRunAIKeepsFarAttackWhenOutOfControl(t *testing.T) {
	owner := actor(1)
	owner.denyAction = true
	far := actor(2)
	far.x = 2000
	owner.known = map[int32]bool{far.ObjectID(): true}
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{canAttack: true})
	addAttackHate(ai, far, 0, 20)

	ai.RunAI()

	if !ai.Desires().Has(&Desire{Kind: IntentionAttack, FinalTarget: far}) {
		t.Fatal("far ATTACK desire pruned while out of control")
	}
	if got := ai.CurrentIntention(); got != IntentionAttack {
		t.Fatalf("CurrentIntention() = %v, want %v", got, IntentionAttack)
	}
}

func TestAttackableThinkDropsCurrentAttackWhenTargetMovesBeyond1500(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	owner.known = map[int32]bool{target.ObjectID(): true}
	strike := &recordingAttack{canAttack: true}
	ai := NewAttackable(owner, &recordingMove{}, strike)
	addAttackHate(ai, target, 0, 20)
	ai.RunAI()
	if strike.target != target {
		t.Fatalf("RunAI attacked = %v, want target", strike.target)
	}

	target.x = 2000
	strike.target = nil
	ai.Think()

	if strike.target != nil {
		t.Fatalf("second Think attacked = %v, want none after distance prune", strike.target)
	}
	if got := ai.CurrentIntention(); got != IntentionIdle {
		t.Fatalf("CurrentIntention() = %v, want %v", got, IntentionIdle)
	}
	if ai.Desires().Has(&Desire{Kind: IntentionAttack, FinalTarget: target}) {
		t.Fatal("ATTACK desire still queued after target moved beyond 1500")
	}
}

func TestAttackableAITickRefreshesStaleThreatAndHate(t *testing.T) {
	owner := actor(1)
	lost := actor(2)
	dead := actor(3)
	dead.alikeDead = true
	kept := actor(4)
	owner.known = map[int32]bool{lost.ObjectID(): false, dead.ObjectID(): true, kept.ObjectID(): true}
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})
	addAttackHate(ai, lost, 7, 70)
	addAttackHate(ai, dead, 8, 80)
	addAttackHate(ai, kept, 9, 90)
	ai.AddHate(lost, 700)
	ai.AddHate(dead, 800)
	ai.AddHate(kept, 900)

	ai.Tick()
	ai.Tick()
	ai.Tick()

	if _, ok := ai.Threats().Get(lost); ok {
		t.Fatal("lost threat entry still present after refresh")
	}
	gotDead, ok := ai.Threats().Get(dead)
	if !ok {
		t.Fatal("dead threat entry was dropped, want damage preserved")
	}
	if gotDead.Hate != -6.6 || gotDead.Damage != 8 {
		t.Fatalf("dead threat entry = %+v, want hate refreshed then decayed and damage preserved", gotDead)
	}
	if got := ai.Threats().Hate(kept); math.Abs(got-83.4) > 0.000001 {
		t.Fatalf("kept threat hate = %v, want decay after refresh", got)
	}
	if got := ai.Hates().Hate(lost); got != 0 {
		t.Fatalf("lost hate entry = %v, want removed", got)
	}
	if got := ai.Hates().Hate(dead); got != 0 {
		t.Fatalf("dead hate entry = %v, want removed", got)
	}
	if got := ai.Hates().Hate(kept); got != 900 {
		t.Fatalf("kept hate entry = %v, want unchanged", got)
	}
}

func armOOTSweep(ai *Attackable, owner *fakeActor) {
	owner.inTerritory = false
	ai.Arrived()
}

func tickOOTSweepDue(ai *Attackable, at time.Time) {
	ai.now = func() time.Time { return at }
	ai.Tick()
}

// TestAttackableAITickClearsStaleThreatOutOfTerritory: a threat entry
// whose last damage is at least staleThreatAge old gets its hate stopped
// and its queued attack desire dropped on the first due firing after an
// out-of-territory arrival arms the sweep.
func TestAttackableAITickClearsStaleThreatOutOfTerritory(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})
	addAttackHate(ai, target, 0, 20)

	start := time.Now()
	ai.now = func() time.Time { return start }
	armOOTSweep(ai, owner)
	tickOOTSweepDue(ai, start.Add(91*time.Second))

	if got := ai.Threats().Hate(target); got != 0 {
		t.Fatalf("hate after stale sweep = %v, want 0 (stopped)", got)
	}
	if _, ok := ai.Threats().Get(target); !ok {
		t.Fatal("threat entry dropped, want kept with hate stopped")
	}
	if got := ai.Desires().Len(); got != 0 {
		t.Fatalf("desires len = %d, want 0 (stale attack desire dropped)", got)
	}
}

// TestAttackableAITickKeepsFreshThreatOutOfTerritory confirms the sweep
// only touches entries whose last damage is stale; an attacker still
// dealing damage within staleThreatAge keeps its desire queued.
func TestAttackableAITickKeepsFreshThreatOutOfTerritory(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})
	addAttackHate(ai, target, 0, 20)

	start := time.Now()
	ai.now = func() time.Time { return start }
	armOOTSweep(ai, owner)
	tickOOTSweepDue(ai, start.Add(10*time.Second))

	if got := ai.Desires().Len(); got != 1 {
		t.Fatalf("desires len = %d, want 1 (fresh attack desire kept)", got)
	}
}

// TestAttackableAITickSkipsStaleSweepInTerritory confirms the sweep never
// runs while the owner is in its territory.
func TestAttackableAITickSkipsStaleSweepInTerritory(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})
	addAttackHate(ai, target, 0, 20)

	start := time.Now()
	ai.now = func() time.Time { return start }
	ai.Arrived()
	tickOOTSweepDue(ai, start.Add(91*time.Second))

	if got := ai.Desires().Len(); got != 1 {
		t.Fatalf("desires len = %d, want 1 (in-territory owner never runs the OOT sweep)", got)
	}
}

// TestAttackableAITickSkipsStaleSweepWithoutArrival confirms a stationary
// out-of-territory owner never arms the sweep, so stale hate is kept.
func TestAttackableAITickSkipsStaleSweepWithoutArrival(t *testing.T) {
	owner := actor(1)
	owner.inTerritory = false
	target := actor(2)
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})
	addAttackHate(ai, target, 0, 1000)

	future := time.Now().Add(91 * time.Second)
	ai.now = func() time.Time { return future }
	for range 10 {
		ai.Tick()
	}

	if got := ai.Desires().Len(); got != 1 {
		t.Fatalf("desires len = %d, want 1 (no arrival means no OOT sweep)", got)
	}
}

// TestAttackableAITickOutOfTerritorySweepKeepsPhaseAcrossInTerritoryTicks
// confirms an in-territory Tick skips that firing but does not cancel the
// schedule, so a later out-of-territory Tick still sweeps.
func TestAttackableAITickOutOfTerritorySweepKeepsPhaseAcrossInTerritoryTicks(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})
	addAttackHate(ai, target, 0, 1000)

	start := time.Now()
	ai.now = func() time.Time { return start }
	armOOTSweep(ai, owner)

	owner.inTerritory = true
	tickOOTSweepDue(ai, start.Add(ootSweepInitialDelay))
	if got := ai.Desires().Len(); got != 1 {
		t.Fatalf("desires len after in-territory firing = %d, want 1", got)
	}

	owner.inTerritory = false
	tickOOTSweepDue(ai, start.Add(91*time.Second))

	if got := ai.Desires().Len(); got != 0 {
		t.Fatalf("desires len = %d, want 0 (phase kept across in-territory tick)", got)
	}
}

// TestAttackableAIArrivedInTerritoryCancelsOOTSweep confirms only an
// in-territory arrival cancels the schedule; later out-of-territory ticks
// without a new arrival do not sweep.
func TestAttackableAIArrivedInTerritoryCancelsOOTSweep(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})
	addAttackHate(ai, target, 0, 1000)

	start := time.Now()
	ai.now = func() time.Time { return start }
	armOOTSweep(ai, owner)

	owner.inTerritory = true
	ai.Arrived()
	owner.inTerritory = false
	tickOOTSweepDue(ai, start.Add(91*time.Second))

	if got := ai.Desires().Len(); got != 1 {
		t.Fatalf("desires len = %d, want 1 (in-territory arrival cancelled the sweep)", got)
	}
}

func TestAttackableAIAddDefaultHateUsesTerritoryOpeningValue(t *testing.T) {
	owner := actor(1)
	first := actor(2)
	second := actor(3)
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})

	ai.AddDefaultHate(first)
	ai.AddDefaultHate(second)

	if got := ai.Hates().Hate(first); got != 300 {
		t.Fatalf("first default hate = %v, want 300", got)
	}
	if got := ai.Hates().Hate(second); got != 100 {
		t.Fatalf("second default hate = %v, want 100", got)
	}
}

func TestAttackableAIAddDefaultHateOutsideTerritoryUsesBaseValue(t *testing.T) {
	owner := actor(1)
	owner.inTerritory = false
	target := actor(2)
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})

	ai.AddDefaultHate(target)

	if got := ai.Hates().Hate(target); got != 100 {
		t.Fatalf("default hate outside territory = %v, want 100", got)
	}
}

func TestAttackableAISetBackToPeaceClearsCombatState(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	owner.known = map[int32]bool{target.ObjectID(): true}
	owner.inTerritory = false
	move := &recordingMove{}
	ai := NewAttackable(owner, move, &recordingAttack{canAttack: true})

	addAttackHate(ai, target, 5, 20)
	ai.AddHate(target, 30)
	ai.RunAI()

	if got := ai.CurrentIntention(); got != IntentionAttack {
		t.Fatalf("CurrentIntention() before reset = %v, want %v", got, IntentionAttack)
	}

	ai.SetBackToPeace()

	if !ai.Threats().IsEmpty() {
		t.Fatal("threat table not cleared")
	}
	if !ai.Hates().IsEmpty() {
		t.Fatal("hate table not cleared")
	}
	if got := ai.Desires().Len(); got != 0 {
		t.Fatalf("desires len = %d, want 0", got)
	}
	if got := ai.CurrentIntention(); got != IntentionIdle {
		t.Fatalf("CurrentIntention() after reset = %v, want %v", got, IntentionIdle)
	}
	if _, _, ok := ai.NextIntention(); ok {
		t.Fatal("NextIntention() ok = true after reset, want false")
	}
	if move.stopCount != 2 {
		t.Fatalf("stop count = %d, want 2", move.stopCount)
	}
}

func TestAttackableAIReduceAllAggroHateReturnsToPeaceWhenExhausted(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	owner.known = map[int32]bool{target.ObjectID(): true}
	move := &recordingMove{}
	ai := NewAttackable(owner, move, &recordingAttack{canAttack: true})

	addAttackHate(ai, target, 0, 5)
	ai.RunAI()
	if got := ai.CurrentIntention(); got != IntentionAttack {
		t.Fatalf("CurrentIntention() before decay = %v, want %v", got, IntentionAttack)
	}

	ai.ReduceAllAggroHate(10)

	if got := ai.CurrentIntention(); got != IntentionIdle {
		t.Fatalf("CurrentIntention() after hate exhausted = %v, want %v", got, IntentionIdle)
	}
	if got := ai.Desires().Len(); got != 0 {
		t.Fatalf("desires len = %d, want 0 after peace", got)
	}
}

func TestAttackableAIStopAggroHateReturnsToPeaceWhenExhausted(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	owner.known = map[int32]bool{target.ObjectID(): true}
	move := &recordingMove{}
	ai := NewAttackable(owner, move, &recordingAttack{canAttack: true})

	addAttackHate(ai, target, 0, 20)
	ai.RunAI()
	if got := ai.CurrentIntention(); got != IntentionAttack {
		t.Fatalf("CurrentIntention() before stop = %v, want %v", got, IntentionAttack)
	}

	ai.StopAggroHate(target)

	if got := ai.CurrentIntention(); got != IntentionIdle {
		t.Fatalf("CurrentIntention() after stop hate = %v, want %v", got, IntentionIdle)
	}
	if got := ai.Desires().Len(); got != 0 {
		t.Fatalf("desires len = %d, want 0 after peace", got)
	}
}

func TestAttackableAIReduceAllAggroHateKeepsAttackWhenSkillHateRemains(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	owner.known = map[int32]bool{target.ObjectID(): true}
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{canAttack: true})

	addAttackHate(ai, target, 0, 5)
	ai.AddHate(target, 50)
	ai.RunAI()
	if got := ai.CurrentIntention(); got != IntentionAttack {
		t.Fatalf("CurrentIntention() before decay = %v, want %v", got, IntentionAttack)
	}

	ai.ReduceAllAggroHate(10)

	if got := ai.CurrentIntention(); got != IntentionAttack {
		t.Fatalf("CurrentIntention() with skill hate remaining = %v, want %v", got, IntentionAttack)
	}
}

func TestAttackableAIRandomizeHateDisplacesTargetAndRebuildsDesires(t *testing.T) {
	owner := actor(1)
	low := actor(2)
	high := actor(3)
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})

	addAttackHate(ai, low, 0, 10)
	addAttackHate(ai, high, 0, 25)

	always := func(attackable.Combatant) bool { return true }
	first := func(int) int { return 0 }
	if ok := ai.RandomizeHate(always, first); !ok {
		t.Fatal("RandomizeHate: ok = false, want true")
	}

	if got := ai.Threats().Hate(low); got != 225 {
		t.Fatalf("displaced attacker hate = %v, want 225", got)
	}
	if got := ai.Threats().Hate(high); got != 25 {
		t.Fatalf("mostHated hate = %v, want unchanged 25", got)
	}

	desire, ok := ai.Desires().Peek()
	if !ok {
		t.Fatal("Desires().Peek() ok = false after RandomizeHate")
	}
	if desire.FinalTarget != low || desire.Weight != 225 {
		t.Fatalf("top desire = (%v, %v), want (low, 225)", desire.FinalTarget, desire.Weight)
	}
	if got := ai.Desires().Len(); got != 2 {
		t.Fatalf("desires len = %d, want 2 (requeued from threat table)", got)
	}
}

// TestAttackableAIAddDamageHateSetsMoveToTarget ports NpcAI.java:698-711:
// every addAttackDesire overload reached from the ordinary hate-list path
// (Npc.java:2036's getAI().addAttackDesire) defaults moveToTarget = true.
func TestAttackableAIAddDamageHateSetsMoveToTarget(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})

	addAttackHate(ai, target, 0, 10)

	desire, ok := ai.Desires().Peek()
	if !ok {
		t.Fatal("Desires().Peek() ok = false after AddDamageHate")
	}
	if !desire.MoveToTarget {
		t.Fatal("AddDamageHate queued desire MoveToTarget = false, want true")
	}
}

// TestAttackableAIAddAttackDesireHoldSetsMoveToTarget ports
// NpcAI.java:683-696: addAttackDesireHold queues MoveToTarget = false.
func TestAttackableAIAddAttackDesireHoldSetsMoveToTarget(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})

	ai.AddDamageHate(target, 0, 10)
	ai.AddAttackDesireHold(target, 10)

	desire, ok := ai.Desires().Peek()
	if !ok {
		t.Fatal("Desires().Peek() ok = false after AddAttackDesireHold")
	}
	if desire.MoveToTarget {
		t.Fatal("AddAttackDesireHold queued desire MoveToTarget = true, want false")
	}
	if desire.FinalTarget != target || desire.Weight != 10 {
		t.Fatalf("hold desire = (%v, %v), want (target, 10)", desire.FinalTarget, desire.Weight)
	}
}

// TestAttackableAIRandomizeHateRequeuesWithMoveToTarget ports
// AggroList.randomizeAttack()'s post-swap requeue loop (AggroList.java:225-226),
// which resolves to the same moveToTarget = true overload.
func TestAttackableAIRandomizeHateRequeuesWithMoveToTarget(t *testing.T) {
	owner := actor(1)
	low := actor(2)
	high := actor(3)
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})

	addAttackHate(ai, low, 0, 10)
	addAttackHate(ai, high, 0, 25)

	always := func(attackable.Combatant) bool { return true }
	first := func(int) int { return 0 }
	if ok := ai.RandomizeHate(always, first); !ok {
		t.Fatal("RandomizeHate: ok = false, want true")
	}

	desire, ok := ai.Desires().Peek()
	if !ok {
		t.Fatal("Desires().Peek() ok = false after RandomizeHate")
	}
	if !desire.MoveToTarget {
		t.Fatal("RandomizeHate requeued desire MoveToTarget = false, want true")
	}
}

func TestAttackableAIRandomizeHateNoopWithSingleAttacker(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})

	addAttackHate(ai, target, 0, 10)

	always := func(attackable.Combatant) bool { return true }
	first := func(int) int { return 0 }
	if ok := ai.RandomizeHate(always, first); ok {
		t.Fatal("RandomizeHate: ok = true, want false with a single attacker")
	}
	if got := ai.Desires().Len(); got != 1 {
		t.Fatalf("desires len = %d, want 1 (untouched)", got)
	}
}

func TestAttackableAIReconsiderTargetSwapsAndDropsPreviousDesire(t *testing.T) {
	owner := actor(1)
	low := actor(2)
	high := actor(3)
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})

	addAttackHate(ai, low, 0, 10)
	addAttackHate(ai, high, 0, 25)

	always := func(attackable.Combatant) bool { return true }
	chosen, ok := ai.ReconsiderTarget(always, always)
	if !ok {
		t.Fatal("ReconsiderTarget: ok = false, want true")
	}
	if chosen != low {
		t.Fatalf("chosen = %v, want low", chosen)
	}

	if got := ai.Threats().Hate(low); got != 10 {
		t.Fatalf("chosen hate = %v, want unchanged 10", got)
	}
	if got := ai.Threats().Hate(high); got != 0 {
		t.Fatalf("previous mostHated hate = %v, want zeroed 0", got)
	}
	if _, ok := ai.Desires().Peek(); !ok {
		t.Fatal("Desires().Peek() ok = false, want the new target's desire queued")
	}
	if got := ai.Desires().Len(); got != 1 {
		t.Fatalf("desires len = %d, want 1 (previous mostHated's desire dropped)", got)
	}
	desire, _ := ai.Desires().Peek()
	if desire.FinalTarget != low {
		t.Fatalf("queued desire target = %v, want low", desire.FinalTarget)
	}
	if desire.Weight != 10 {
		t.Fatalf("queued desire weight = %v, want 10 (unchanged from AddDamageHate; "+
			"AggroList.java:169-177 never re-queues the chosen's desire, so "+
			"ReconsiderTarget must not double it via AddOrUpdate's accumulation)", desire.Weight)
	}
}

func TestAttackableAIReconsiderTargetNoopWithSingleAttacker(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})

	addAttackHate(ai, target, 0, 10)

	always := func(attackable.Combatant) bool { return true }
	if _, ok := ai.ReconsiderTarget(always, always); ok {
		t.Fatal("ReconsiderTarget: ok = true, want false with a single attacker")
	}
	if got := ai.Desires().Len(); got != 1 {
		t.Fatalf("desires len = %d, want 1 (untouched)", got)
	}
}

// ---- from attackable_wander_test.go ----
func TestAttackableAIPromotesMoveToDesireAndIdlesOnArrival(t *testing.T) {
	owner := actor(1)
	owner.x, owner.y, owner.z = 100, 0, 0
	move := &recordingMove{}
	brain := NewAttackable(owner, move, &recordingAttack{})
	home := location.Location{}

	brain.AddMoveToDesire(home, 1_000_000)
	if err := brain.RunAI(); err != nil {
		t.Fatalf("RunAI() error: %v", err)
	}
	if got := brain.CurrentIntention(); got != IntentionMoveTo {
		t.Fatalf("CurrentIntention() after RunAI = %v, want %v", got, IntentionMoveTo)
	}
	if move.home != home {
		t.Fatalf("MoveHome destination = %#v, want %#v", move.home, home)
	}

	owner.x, owner.y, owner.z = home.X, home.Y, home.Z
	if err := brain.Think(); err != nil {
		t.Fatalf("Think() after arrival error: %v", err)
	}
	if got := brain.CurrentIntention(); got != IntentionIdle {
		t.Fatalf("CurrentIntention() after arrival = %v, want %v", got, IntentionIdle)
	}
}

// hitAnimationAttack is a recordingAttack that reports a hit animation
// window.
type hitAnimationAttack struct {
	recordingAttack
	inHitAnimation bool
}

func (a *hitAnimationAttack) InHitAnimation() bool { return a.inHitAnimation }

// TestAttackableAIHoldsPromotionInsideHitAnimation pins NpcAI.runAI's hit
// animation gate: a queued desire is not taken up while the attack
// controller reports its hit animation window open, and is taken up by the
// next pass once the window closes.
func TestAttackableAIHoldsPromotionInsideHitAnimation(t *testing.T) {
	owner := actor(1)
	owner.x, owner.y, owner.z = 100, 0, 0
	move := &recordingMove{}
	strike := &hitAnimationAttack{inHitAnimation: true}
	brain := NewAttackable(owner, move, strike)
	dest := location.Location{X: 300}
	unmoved := location.Location{X: -1, Y: -1, Z: -1}
	move.home = unmoved

	brain.AddMoveToDesire(dest, 1_000_000)
	if err := brain.RunAI(); err != nil {
		t.Fatalf("RunAI() inside the hit animation error: %v", err)
	}
	if got := brain.CurrentIntention(); got != IntentionIdle {
		t.Fatalf("CurrentIntention() inside the hit animation = %v, want %v", got, IntentionIdle)
	}
	if move.home != unmoved {
		t.Fatalf("MoveHome destination inside the hit animation = %#v, want no move", move.home)
	}

	strike.inHitAnimation = false
	if err := brain.RunAI(); err != nil {
		t.Fatalf("RunAI() after the hit animation error: %v", err)
	}
	if got := brain.CurrentIntention(); got != IntentionMoveTo {
		t.Fatalf("CurrentIntention() after the hit animation = %v, want %v", got, IntentionMoveTo)
	}
	if move.home != dest {
		t.Fatalf("MoveHome destination after the hit animation = %#v, want %#v", move.home, dest)
	}
}

func TestAttackableAIArrivedClearsMoveToWhenGeoSnapsZ(t *testing.T) {
	owner := actor(1)
	owner.x, owner.y, owner.z = 100, 0, 0
	move := &recordingMove{}
	brain := NewAttackable(owner, move, &recordingAttack{})
	home := location.Location{}

	brain.AddMoveToDesire(home, 1_000_000)
	if err := brain.RunAI(); err != nil {
		t.Fatalf("RunAI() error: %v", err)
	}
	if got := brain.CurrentIntention(); got != IntentionMoveTo {
		t.Fatalf("CurrentIntention() after RunAI = %v, want %v", got, IntentionMoveTo)
	}

	// |ΔZ| > Desire.Equal's 30 so thinkMoveTo's fast path cannot idle.
	owner.x, owner.y, owner.z = home.X, home.Y, home.Z-40
	brain.Arrived()
	move.home = location.Location{X: -1, Y: -1, Z: -1}
	if err := brain.Think(); err != nil {
		t.Fatalf("Think() after Arrived error: %v", err)
	}
	if got := brain.CurrentIntention(); got != IntentionIdle {
		t.Fatalf("CurrentIntention() after Arrived = %v, want %v", got, IntentionIdle)
	}
	if move.home == home {
		t.Fatal("Think after Arrived restarted MoveHome")
	}
}

func TestAttackableAIArrivedBlockedClearsMoveTo(t *testing.T) {
	owner := actor(1)
	owner.x, owner.y, owner.z = 100, 0, 0
	move := &recordingMove{}
	brain := NewAttackable(owner, move, &recordingAttack{})
	home := location.Location{}

	brain.AddMoveToDesire(home, 1_000_000)
	if err := brain.RunAI(); err != nil {
		t.Fatalf("RunAI() error: %v", err)
	}

	brain.ArrivedBlocked()
	move.home = location.Location{X: -1, Y: -1, Z: -1}
	if err := brain.Think(); err != nil {
		t.Fatalf("Think() after ArrivedBlocked error: %v", err)
	}
	if got := brain.CurrentIntention(); got != IntentionIdle {
		t.Fatalf("CurrentIntention() after ArrivedBlocked = %v, want %v", got, IntentionIdle)
	}
	if move.home == home {
		t.Fatal("Think after ArrivedBlocked restarted MoveHome")
	}
}

func TestAttackableAIArrivedIdlesEvenWhileAttackingNow(t *testing.T) {
	owner := actor(1)
	owner.x, owner.y, owner.z = 100, 0, 0
	mv := &recordingMove{}
	atk := &recordingAttack{attackingNow: true}
	brain := NewAttackable(owner, mv, atk)
	home := location.Location{}

	brain.AddMoveToDesire(home, 1_000_000)
	if err := brain.RunAI(); err != nil {
		t.Fatalf("RunAI() error: %v", err)
	}
	if got := brain.CurrentIntention(); got != IntentionMoveTo {
		t.Fatalf("CurrentIntention() after RunAI = %v, want %v", got, IntentionMoveTo)
	}

	owner.x, owner.y, owner.z = home.X, home.Y, home.Z-40
	brain.Arrived()
	mv.home = location.Location{X: -1, Y: -1, Z: -1}
	if err := brain.Think(); err != nil {
		t.Fatalf("Think() after Arrived error: %v", err)
	}
	if got := brain.CurrentIntention(); got != IntentionIdle {
		t.Fatalf("CurrentIntention() after Arrived while attacking = %v, want idle", got)
	}
	if mv.home == home {
		t.Fatal("Think after Arrived restarted MoveHome while attacking")
	}
}

func TestAttackableAIArrivedRestoresSpawnHeading(t *testing.T) {
	owner := actor(1)
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})

	ai.Arrived()

	if owner.headingRestores != 1 {
		t.Fatalf("heading restores = %d, want 1", owner.headingRestores)
	}
}

func TestAttackableAIFollowArrivedSkipsSpawnHeadingAndOOTSweep(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})
	ai.Desires().AddOrUpdate(&Desire{Kind: IntentionFollow, FinalTarget: target, Weight: 5})
	if err := ai.RunAI(); err != nil {
		t.Fatalf("RunAI() error: %v", err)
	}
	if got := ai.CurrentIntention(); got != IntentionFollow {
		t.Fatalf("CurrentIntention() = %v, want follow", got)
	}

	addAttackHate(ai, target, 0, 20)
	start := time.Now()
	ai.now = func() time.Time { return start }
	owner.inTerritory = false
	ai.Arrived()

	if owner.headingRestores != 0 {
		t.Fatalf("heading restores = %d, want 0 on FOLLOW arrival", owner.headingRestores)
	}

	tickOOTSweepDue(ai, start.Add(91*time.Second))
	if got := ai.Desires().Has(&Desire{Kind: IntentionAttack, FinalTarget: target}); !got {
		t.Fatal("FOLLOW arrival armed the OOT sweep, want it skipped")
	}
}

func TestAttackableAIAttackArrivedRestoresHeadingAndArmsOOTSweep(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	owner.known = map[int32]bool{target.ObjectID(): true}
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{canAttack: true})
	addAttackHate(ai, target, 0, 1000)
	if err := ai.RunAI(); err != nil {
		t.Fatalf("RunAI() error: %v", err)
	}
	if got := ai.CurrentIntention(); got != IntentionAttack {
		t.Fatalf("CurrentIntention() = %v, want attack", got)
	}

	start := time.Now()
	ai.now = func() time.Time { return start }
	owner.inTerritory = false
	ai.Arrived()

	if owner.headingRestores != 1 {
		t.Fatalf("heading restores = %d, want 1 on ATTACK arrival", owner.headingRestores)
	}

	tickOOTSweepDue(ai, start.Add(91*time.Second))
	if got := ai.Desires().Has(&Desire{Kind: IntentionAttack, FinalTarget: target}); got {
		t.Fatal("ATTACK arrival did not arm the OOT sweep")
	}
}

func TestAttackableAIAddMoveToDesireSkipsUnreachable(t *testing.T) {
	owner := actor(1)
	move := &recordingMove{denyMove: true}
	brain := NewAttackable(owner, move, &recordingAttack{})

	if brain.AddMoveToDesire(location.Location{X: 50, Y: 0, Z: 0}, 1_000_000) {
		t.Fatal("AddMoveToDesire() = true, want false when unreachable")
	}
	if got := brain.Desires().Len(); got != 0 {
		t.Fatalf("queued desires = %d, want 0", got)
	}
}

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

	if got := ai.CurrentIntention(); got != IntentionIdle {
		t.Fatalf("CurrentIntention() = %v, want idle outside territory without return home", got)
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

func TestAttackableAIIdleQueuesWanderAndWalks(t *testing.T) {
	owner := actor(1)
	owner.idleWander = true
	owner.moveSpeed = 40
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})

	if err := tickThinkIdle(ai); err != nil {
		t.Fatalf("TickThink() error: %v", err)
	}
	if got := ai.CurrentIntention(); got != IntentionIdle {
		t.Fatalf("CurrentIntention() after idle abort = %v, want idle", got)
	}
	got, ok := ai.Desires().Peek()
	if !ok || got.Kind != IntentionWander || got.Timer != 5 || got.Weight != 5 {
		t.Fatalf("queued wander = (%v %+v), want timer 5 weight 5", ok, got)
	}
	if owner.wanderCalls != 0 {
		t.Fatalf("wander move = %d after queue tick, want 0 (promote next cycle)", owner.wanderCalls)
	}

	if err := ai.TickThink(); err != nil {
		t.Fatalf("promote TickThink() error: %v", err)
	}
	if got := ai.CurrentIntention(); got != IntentionWander {
		t.Fatalf("CurrentIntention() = %v, want wander", got)
	}
	if owner.wanderCalls != 1 || owner.wanderOffset != 120 {
		t.Fatalf("wander move = %d offset %d, want 1 call offset 120", owner.wanderCalls, owner.wanderOffset)
	}
}

func TestAttackableAIFirstTickDoesNotPromoteWander(t *testing.T) {
	owner := actor(1)
	owner.moveSpeed = 40
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})
	ai.Desires().AddOrUpdate(&Desire{Kind: IntentionWander, Timer: 5, Weight: 5})

	if err := ai.TickThink(); err != nil {
		t.Fatalf("TickThink() error: %v", err)
	}
	if got := ai.CurrentIntention(); got != IntentionIdle {
		t.Fatalf("CurrentIntention() after first TickThink = %v, want idle", got)
	}
	if owner.wanderCalls != 0 {
		t.Fatalf("wander move = %d on first TickThink, want 0", owner.wanderCalls)
	}
	if !ai.Desires().Has(&Desire{Kind: IntentionWander}) {
		t.Fatal("wander desire dropped on first TickThink, want it kept")
	}

	if err := ai.TickThink(); err != nil {
		t.Fatalf("second TickThink() error: %v", err)
	}
	if got := ai.CurrentIntention(); got != IntentionWander {
		t.Fatalf("CurrentIntention() after second TickThink = %v, want wander", got)
	}
	if owner.wanderCalls != 1 {
		t.Fatalf("wander move = %d after second TickThink, want 1", owner.wanderCalls)
	}
}

func TestAttackableAIFirstTickPromotesWhenAttackQueued(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	owner.known = map[int32]bool{target.ObjectID(): true}
	strike := &recordingAttack{canAttack: true}
	ai := NewAttackable(owner, &recordingMove{}, strike)
	ai.Threats().AddDamage(target, 0, 10)
	ai.Desires().AddOrUpdate(&Desire{Kind: IntentionAttack, FinalTarget: target, Weight: 10})

	if err := ai.TickThink(); err != nil {
		t.Fatalf("TickThink() error: %v", err)
	}
	if got := ai.CurrentIntention(); got != IntentionAttack {
		t.Fatalf("CurrentIntention() after first TickThink = %v, want attack", got)
	}
	if strike.target != target {
		t.Fatalf("attacked target = %v, want queued attacker", strike.target)
	}
}

func TestAttackableAIFirstTickPromotesHighestWeightWhenAttackOpensGate(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	owner.known = map[int32]bool{target.ObjectID(): true}
	owner.moveSpeed = 40
	strike := &recordingAttack{canAttack: true}
	ai := NewAttackable(owner, &recordingMove{}, strike)
	ai.Threats().AddDamage(target, 0, 1)
	ai.Desires().AddOrUpdate(&Desire{Kind: IntentionAttack, FinalTarget: target, Weight: 1})
	ai.Desires().AddOrUpdate(&Desire{Kind: IntentionWander, Timer: 5, Weight: 100})

	if err := ai.TickThink(); err != nil {
		t.Fatalf("TickThink() error: %v", err)
	}
	if got := ai.CurrentIntention(); got != IntentionWander {
		t.Fatalf("CurrentIntention() = %v, want wander (highest weight, gate opened by queued attack)", got)
	}
	if strike.target != nil {
		t.Fatalf("attacked target = %v, want none while wander outranks attack", strike.target)
	}
	if owner.wanderCalls != 1 {
		t.Fatalf("wander move = %d, want 1", owner.wanderCalls)
	}
}

func TestAttackableAIFirstTickPromotesAfterAttackDesirePruned(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	owner.known = map[int32]bool{target.ObjectID(): true}
	owner.moveSpeed = 40
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})
	ai.Desires().AddOrUpdate(&Desire{Kind: IntentionAttack, FinalTarget: target, Weight: 10})
	ai.Desires().AddOrUpdate(&Desire{Kind: IntentionWander, Timer: 5, Weight: 5})

	if err := ai.TickThink(); err != nil {
		t.Fatalf("TickThink() error: %v", err)
	}
	if got := ai.CurrentIntention(); got != IntentionWander {
		t.Fatalf("CurrentIntention() = %v, want wander (ATTACK presence latched before prune)", got)
	}
	if owner.wanderCalls != 1 {
		t.Fatalf("wander move = %d, want 1", owner.wanderCalls)
	}
	if ai.Desires().Has(&Desire{Kind: IntentionAttack, FinalTarget: target}) {
		t.Fatal("ATTACK desire still queued after empty-threat prune")
	}
}

func TestAttackableAIDoesNotPromoteWhileCasting(t *testing.T) {
	owner := actor(1)
	owner.moveSpeed = 40
	cast := &recordingCast{casting: true}
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})
	ai.SetCastController(cast)
	ai.lifeTime = 1
	ai.Desires().AddOrUpdate(&Desire{Kind: IntentionWander, Timer: 5, Weight: 5})

	if err := ai.TickThink(); err != nil {
		t.Fatalf("TickThink() error: %v", err)
	}
	if got := ai.CurrentIntention(); got != IntentionIdle {
		t.Fatalf("CurrentIntention() while casting = %v, want idle", got)
	}
	if owner.wanderCalls != 0 {
		t.Fatalf("wander move = %d while casting, want 0", owner.wanderCalls)
	}
}

func TestAttackableAIIdleHoldPositionForcesWalkStance(t *testing.T) {
	owner := actor(1)
	move := &recordingMove{}
	ai := NewAttackable(owner, move, &recordingAttack{})

	if err := tickThinkIdle(ai); err != nil {
		t.Fatalf("TickThink() error: %v", err)
	}

	if got := ai.CurrentIntention(); got != IntentionIdle {
		t.Fatalf("CurrentIntention() = %v, want idle (no wander queue)", got)
	}
	if owner.walkStanceCalls != 1 {
		t.Fatalf("walk stance calls = %d, want 1", owner.walkStanceCalls)
	}
	if move.stopCount != 1 {
		t.Fatalf("stop count = %d, want 1", move.stopCount)
	}
}

func TestAttackableAIIdleAbortsInFlightAttackWhenQueueEmpty(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	owner.known = map[int32]bool{target.ObjectID(): true}
	move := &recordingMove{}
	strike := &recordingAttack{canAttack: true, attackingNow: true}
	ai := NewAttackable(owner, move, strike)
	addAttackHate(ai, target, 0, 20)
	if err := ai.RunAI(); err != nil {
		t.Fatalf("RunAI() error: %v", err)
	}
	if got := ai.CurrentIntention(); got != IntentionAttack {
		t.Fatalf("CurrentIntention() = %v, want attack", got)
	}

	ai.Desires().Clear()
	strike.attackingNow = true
	if err := ai.Think(); err != nil {
		t.Fatalf("empty-queue Think() error: %v", err)
	}
	if strike.stopCalls != 0 {
		t.Fatalf("attack Stop calls = %d after event Think, want 0", strike.stopCalls)
	}

	if err := tickThinkIdle(ai); err != nil {
		t.Fatalf("TickThink() error: %v", err)
	}

	if strike.stopCalls != 1 {
		t.Fatalf("attack Stop calls = %d, want 1", strike.stopCalls)
	}
	if owner.walkStanceCalls != 1 {
		t.Fatalf("walk stance calls = %d, want 1", owner.walkStanceCalls)
	}
	if got := ai.CurrentIntention(); got != IntentionIdle {
		t.Fatalf("CurrentIntention() = %v, want idle after empty-queue abort", got)
	}
}

func TestAttackableAIArrivedThinkDoesNotAbortInFlightAttack(t *testing.T) {
	owner := actor(1)
	move := &recordingMove{}
	strike := &recordingAttack{attackingNow: true}
	ai := NewAttackable(owner, move, strike)
	if err := thinkWanderOnce(ai); err != nil {
		t.Fatalf("Think() error: %v", err)
	}
	ai.Arrived()
	stops := strike.stopCalls
	walk := owner.walkStanceCalls
	if err := ai.Think(); err != nil {
		t.Fatalf("arrival Think() error: %v", err)
	}
	if strike.stopCalls != stops {
		t.Fatalf("attack Stop calls = %d on arrival Think, want %d", strike.stopCalls, stops)
	}
	if owner.walkStanceCalls != walk {
		t.Fatalf("walk stance calls = %d on arrival Think, want %d", owner.walkStanceCalls, walk)
	}

	if err := tickThinkIdle(ai); err != nil {
		t.Fatalf("TickThink() error: %v", err)
	}
	if strike.stopCalls != stops+1 {
		t.Fatalf("attack Stop calls = %d after TickThink, want %d", strike.stopCalls, stops+1)
	}
}

func TestAttackableAIIdleSkipsAbortWhileCasting(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	owner.known = map[int32]bool{target.ObjectID(): true}
	move := &recordingMove{}
	strike := &recordingAttack{canAttack: true}
	cast := &recordingCast{canAttempt: true, canCast: true, casting: true}
	ai := NewAttackable(owner, move, strike)
	ai.SetCastController(cast)
	ai.Desires().AddOrUpdate(&Desire{Kind: IntentionCast, FinalTarget: target, Skill: skill.Ref{ID: 4, Level: 1}, Weight: 10})
	if err := ai.RunAI(); err != nil {
		t.Fatalf("RunAI() error: %v", err)
	}

	ai.Desires().Clear()
	cast.casting = true
	walkBefore := owner.walkStanceCalls
	if err := tickThinkIdle(ai); err != nil {
		t.Fatalf("casting TickThink() error: %v", err)
	}
	if strike.stopCalls != 0 {
		t.Fatalf("attack Stop calls = %d, want 0 while casting", strike.stopCalls)
	}
	if cast.stopCalls != 0 {
		t.Fatalf("cast Stop calls = %d, want 0 while casting", cast.stopCalls)
	}
	if owner.walkStanceCalls != walkBefore {
		t.Fatalf("walk stance calls = %d, want %d while casting", owner.walkStanceCalls, walkBefore)
	}
}

func TestAttackableAIWanderTimerThenRateWalks(t *testing.T) {
	owner := actor(1)
	owner.moveSpeed = 50
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	now := start
	ai.now = func() time.Time { return now }
	ai.SetRandomWalkRate(100)
	ai.roll = func(int) int { return 0 }

	if err := thinkWanderOnce(ai); err != nil {
		t.Fatalf("first RunAI() error: %v", err)
	}
	owner.wanderCalls = 0

	if err := ai.RunAI(); err != nil {
		t.Fatalf("arm-timer RunAI() error: %v", err)
	}
	if owner.wanderCalls != 0 {
		t.Fatalf("wander calls while timer arms = %d, want 0", owner.wanderCalls)
	}

	now = start.Add(4 * time.Second)
	if err := ai.RunAI(); err != nil {
		t.Fatalf("early RunAI() error: %v", err)
	}
	if owner.wanderCalls != 0 {
		t.Fatalf("wander calls before timer = %d, want 0", owner.wanderCalls)
	}

	now = start.Add(5 * time.Second)
	if err := ai.RunAI(); err != nil {
		t.Fatalf("due RunAI() error: %v", err)
	}
	if owner.wanderCalls != 1 {
		t.Fatalf("wander calls after timer + rate = %d, want 1", owner.wanderCalls)
	}
}

func TestAttackableAIWanderRateZeroReschedulesWithoutWalking(t *testing.T) {
	owner := actor(1)
	owner.moveSpeed = 50
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	now := start
	ai.now = func() time.Time { return now }
	ai.SetRandomWalkRate(0)

	if err := thinkWanderOnce(ai); err != nil {
		t.Fatalf("first RunAI() error: %v", err)
	}
	owner.wanderCalls = 0
	if err := ai.RunAI(); err != nil {
		t.Fatalf("arm-timer RunAI() error: %v", err)
	}

	now = start.Add(5 * time.Second)
	if err := ai.RunAI(); err != nil {
		t.Fatalf("due RunAI() error: %v", err)
	}
	if owner.wanderCalls != 0 {
		t.Fatalf("wander calls with rate 0 = %d, want 0", owner.wanderCalls)
	}

	now = start.Add(9 * time.Second)
	if err := ai.RunAI(); err != nil {
		t.Fatalf("before second timer RunAI() error: %v", err)
	}
	if owner.wanderCalls != 0 {
		t.Fatalf("wander calls before rescheduled timer = %d, want 0", owner.wanderCalls)
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

// ---- from desire_queue_test.go ----
func TestDesireQueueAddOrUpdateMergesEqualDesireInPlace(t *testing.T) {
	q := NewDesireQueue()
	target := actor(1)

	first := &Desire{Kind: IntentionAttack, FinalTarget: target, Weight: 10}
	q.AddOrUpdate(first)

	second := &Desire{Kind: IntentionAttack, FinalTarget: target, Weight: 5}
	q.AddOrUpdate(second)

	if got := q.Len(); got != 1 {
		t.Fatalf("Len() = %d, want 1 (equal Desires must merge, not accumulate)", got)
	}

	got, ok := q.Peek()
	if !ok {
		t.Fatal("Peek() ok = false, want true")
	}
	if got != first {
		t.Fatalf("Peek() returned %p, want the original queued Desire %p (weight must merge in place, not reallocate)", got, first)
	}
	if got.Weight != 15 {
		t.Fatalf("Weight = %v, want 15 (10 + 5 merged)", got.Weight)
	}
}

func TestDesireQueueAddOrUpdateKeepsDistinctDesiresSeparate(t *testing.T) {
	q := NewDesireQueue()
	low := actor(1)
	high := actor(2)

	q.AddOrUpdate(&Desire{Kind: IntentionAttack, FinalTarget: low, Weight: 10})
	q.AddOrUpdate(&Desire{Kind: IntentionAttack, FinalTarget: high, Weight: 25})

	if got := q.Len(); got != 2 {
		t.Fatalf("Len() = %d, want 2", got)
	}
}

func TestDesireQueuePeekReturnsHighestWeight(t *testing.T) {
	q := NewDesireQueue()
	low := actor(1)
	mid := actor(2)
	high := actor(3)

	q.AddOrUpdate(&Desire{Kind: IntentionAttack, FinalTarget: low, Weight: 10})
	q.AddOrUpdate(&Desire{Kind: IntentionAttack, FinalTarget: high, Weight: 25})
	q.AddOrUpdate(&Desire{Kind: IntentionAttack, FinalTarget: mid, Weight: 15})

	got, ok := q.Peek()
	if !ok {
		t.Fatal("Peek() ok = false, want true")
	}
	if got.FinalTarget != high {
		t.Fatalf("Peek() target = %v, want highest-weight target", got.FinalTarget)
	}
}

func TestDesireQueuePeekEmpty(t *testing.T) {
	q := NewDesireQueue()

	if _, ok := q.Peek(); ok {
		t.Fatal("Peek() ok = true on empty queue, want false")
	}
}

func TestDesireQueueRespectsCapacity(t *testing.T) {
	q := NewDesireQueue()

	for i := int32(0); i < maxDesires+10; i++ {
		q.AddOrUpdate(&Desire{Kind: IntentionAttack, FinalTarget: actor(i), Weight: float64(i)})
	}

	if got := q.Len(); got != maxDesires {
		t.Fatalf("Len() = %d, want %d (capped)", got, maxDesires)
	}

	// A merge into an already-queued Desire must still succeed once the
	// queue is at capacity: capacity only blocks brand-new entries.
	q.AddOrUpdate(&Desire{Kind: IntentionAttack, FinalTarget: actor(0), Weight: 100})
	if got := q.Len(); got != maxDesires {
		t.Fatalf("Len() after merge at capacity = %d, want %d", got, maxDesires)
	}
	got, _ := q.Peek()
	if got.FinalTarget.ObjectID() != 0 || got.Weight != 100 {
		t.Fatalf("Peek() = (%v, %v), want (actor 0, weight 100)", got.FinalTarget, got.Weight)
	}
}

func TestDesireQueueDecreaseWeightByTypeRemovesBelowZero(t *testing.T) {
	q := NewDesireQueue()
	survivor := actor(1)
	victim := actor(2)

	q.AddOrUpdate(&Desire{Kind: IntentionAttack, FinalTarget: survivor, Weight: 10})
	q.AddOrUpdate(&Desire{Kind: IntentionAttack, FinalTarget: victim, Weight: 3})
	q.AddOrUpdate(&Desire{Kind: IntentionWander, Weight: 100})

	q.DecreaseWeightByType(IntentionAttack, 6.6)

	if got := q.Len(); got != 2 {
		t.Fatalf("Len() = %d, want 2 (one ATTACK Desire dropped below zero, WANDER untouched)", got)
	}

	got, ok := q.Peek()
	if !ok {
		t.Fatal("Peek() ok = false, want true")
	}
	if got.Kind != IntentionWander {
		t.Fatalf("Peek() kind = %v, want wander (highest remaining weight)", got.Kind)
	}
}

func TestDesireQueueRemoveByKindAndTarget(t *testing.T) {
	q := NewDesireQueue()
	target := actor(1)
	other := actor(2)
	q.AddOrUpdate(&Desire{Kind: IntentionAttack, FinalTarget: target, Weight: 100})
	q.AddOrUpdate(&Desire{Kind: IntentionCast, FinalTarget: target, Weight: 200})
	q.AddOrUpdate(&Desire{Kind: IntentionAttack, FinalTarget: other, Weight: 50})

	q.Remove(IntentionAttack, target)

	if got := q.Len(); got != 2 {
		t.Fatalf("Len() = %d, want 2", got)
	}
	got, ok := q.Peek()
	if !ok {
		t.Fatal("Peek() ok = false, want true")
	}
	if got.Kind != IntentionCast || got.FinalTarget != target {
		t.Fatalf("Peek() = (%v, %v), want cast desire for removed attack target still present", got.Kind, got.FinalTarget)
	}
}

func TestDesireQueueRemoveFinalTarget(t *testing.T) {
	q := NewDesireQueue()
	target := actor(1)
	other := actor(2)
	q.AddOrUpdate(&Desire{Kind: IntentionAttack, FinalTarget: target, Weight: 100})
	q.AddOrUpdate(&Desire{Kind: IntentionCast, FinalTarget: target, Weight: 200})
	q.AddOrUpdate(&Desire{Kind: IntentionAttack, FinalTarget: other, Weight: 50})

	q.RemoveFinalTarget(target)

	if got := q.Len(); got != 1 {
		t.Fatalf("Len() = %d, want only the other target left", got)
	}
	got, ok := q.Peek()
	if !ok || got.FinalTarget != other {
		t.Fatalf("Peek() = (%v, %v), want other target", got, ok)
	}
}

func TestDesireQueueRemoveKind(t *testing.T) {
	q := NewDesireQueue()
	target := actor(1)
	q.AddOrUpdate(&Desire{Kind: IntentionAttack, FinalTarget: target, Weight: 100})
	q.AddOrUpdate(&Desire{Kind: IntentionCast, FinalTarget: target, Weight: 200})

	q.RemoveKind(IntentionAttack)

	if got := q.Len(); got != 1 {
		t.Fatalf("Len() = %d, want only cast desire left", got)
	}
	got, ok := q.Peek()
	if !ok || got.Kind != IntentionCast {
		t.Fatalf("Peek() = (%v, %v), want cast desire", got, ok)
	}
}

func TestDesireQueueConcurrentAccess(t *testing.T) {
	q := NewDesireQueue()

	var wg sync.WaitGroup
	for i := int32(0); i < 100; i++ {
		wg.Add(1)
		go func(id int32) {
			defer wg.Done()
			target := actor(id % 10)
			q.AddOrUpdate(&Desire{Kind: IntentionAttack, FinalTarget: target, Weight: 10})
			q.Peek()
			q.Len()
			q.DecreaseWeightByType(IntentionAttack, 1)
		}(i)
	}
	wg.Wait()
}

// ---- from desire_test.go ----
func TestDesireEqual(t *testing.T) {
	target := actor(2)
	otherTarget := actor(3)

	tests := []struct {
		name string
		a, b *Desire
		want bool
	}{
		{
			name: "idle always equal",
			a:    &Desire{Kind: IntentionIdle},
			b:    &Desire{Kind: IntentionIdle, Weight: 5},
			want: true,
		},
		{
			name: "wander always equal",
			a:    &Desire{Kind: IntentionWander},
			b:    &Desire{Kind: IntentionWander},
			want: true,
		},
		{
			name: "attack same final target",
			a:    &Desire{Kind: IntentionAttack, FinalTarget: target},
			b:    &Desire{Kind: IntentionAttack, FinalTarget: target},
			want: true,
		},
		{
			name: "attack different final target",
			a:    &Desire{Kind: IntentionAttack, FinalTarget: target},
			b:    &Desire{Kind: IntentionAttack, FinalTarget: otherTarget},
			want: false,
		},
		{
			name: "different kind",
			a:    &Desire{Kind: IntentionAttack, FinalTarget: target},
			b:    &Desire{Kind: IntentionFlee, FinalTarget: target},
			want: false,
		},
		{
			name: "cast requires same target and skill",
			a:    &Desire{Kind: IntentionCast, FinalTarget: target, Skill: skill.Ref{ID: 1, Level: 2}},
			b:    &Desire{Kind: IntentionCast, FinalTarget: target, Skill: skill.Ref{ID: 1, Level: 2}},
			want: true,
		},
		{
			name: "cast rejects different skill level",
			a:    &Desire{Kind: IntentionCast, FinalTarget: target, Skill: skill.Ref{ID: 1, Level: 2}},
			b:    &Desire{Kind: IntentionCast, FinalTarget: target, Skill: skill.Ref{ID: 1, Level: 3}},
			want: false,
		},
		{
			name: "pick up requires same item",
			a:    &Desire{Kind: IntentionPickUp, ItemObjectID: 7},
			b:    &Desire{Kind: IntentionPickUp, ItemObjectID: 7},
			want: true,
		},
		{
			name: "social requires same id",
			a:    &Desire{Kind: IntentionSocial, ItemObjectID: 7},
			b:    &Desire{Kind: IntentionSocial, ItemObjectID: 8},
			want: false,
		},
		{
			name: "move route requires same route name",
			a:    &Desire{Kind: IntentionMoveRoute, RouteName: "patrol"},
			b:    &Desire{Kind: IntentionMoveRoute, RouteName: "patrol"},
			want: true,
		},
		{
			name: "move route rejects different route name",
			a:    &Desire{Kind: IntentionMoveRoute, RouteName: "patrol"},
			b:    &Desire{Kind: IntentionMoveRoute, RouteName: "guard"},
			want: false,
		},
		{
			name: "move to within tolerance",
			a:    &Desire{Kind: IntentionMoveTo, Location: location.Location{X: 0, Y: 0, Z: 0}},
			b:    &Desire{Kind: IntentionMoveTo, Location: location.Location{X: 10, Y: 10, Z: 20}},
			want: true,
		},
		{
			name: "move to beyond ground tolerance",
			a:    &Desire{Kind: IntentionMoveTo, Location: location.Location{X: 0, Y: 0, Z: 0}},
			b:    &Desire{Kind: IntentionMoveTo, Location: location.Location{X: 30, Y: 0, Z: 0}},
			want: false,
		},
		{
			name: "move to beyond height tolerance",
			a:    &Desire{Kind: IntentionMoveTo, Location: location.Location{X: 0, Y: 0, Z: 0}},
			b:    &Desire{Kind: IntentionMoveTo, Location: location.Location{X: 0, Y: 0, Z: 31}},
			want: false,
		},
		{
			name: "interact never merges, even with matching target",
			a:    &Desire{Kind: IntentionInteract, Target: target},
			b:    &Desire{Kind: IntentionInteract, Target: target},
			want: false,
		},
		{
			name: "use item never merges",
			a:    &Desire{Kind: IntentionUseItem, ItemObjectID: 5},
			b:    &Desire{Kind: IntentionUseItem, ItemObjectID: 5},
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.a.Equal(tc.b); got != tc.want {
				t.Errorf("Equal() = %v, want %v", got, tc.want)
			}
			// Equal must be symmetric.
			if got := tc.b.Equal(tc.a); got != tc.want {
				t.Errorf("reverse Equal() = %v, want %v", got, tc.want)
			}
		})
	}
}

// ---- from summon_test.go ----
func TestSummonAITryToAttackExecutesPhysicalAttack(t *testing.T) {
	owner := actor(100)
	target := actor(200)
	move := &summonMove{}
	strike := &recordingAttack{canAttack: true}
	brain := NewSummon(owner, move, strike)

	if !brain.TryToAttack(target) {
		t.Fatal("TryToAttack() = false, want accepted attack")
	}
	if strike.target != target {
		t.Fatalf("attack target = %v, want target", strike.target)
	}
	if move.followTarget != target || move.followRange != owner.attackRange {
		t.Fatalf("offensive follow = (%v, %d), want (%v, %d)", move.followTarget, move.followRange, target, owner.attackRange)
	}
	if got := brain.CurrentIntention(); got != IntentionAttack {
		t.Fatalf("CurrentIntention() = %v, want attack", got)
	}
}

func TestSummonAITryToAttackQueuesWhileSwingingAndExecutesOnThink(t *testing.T) {
	owner := actor(100)
	target := actor(200)
	strike := &recordingAttack{canAttack: true, attackingNow: true}
	brain := NewSummon(owner, &summonMove{}, strike)

	if !brain.TryToAttack(target) {
		t.Fatal("TryToAttack() = false, want queued attack accepted while already attacking")
	}
	if strike.target != nil {
		t.Fatalf("attack target = %v while busy, want queued without a new swing", strike.target)
	}
	if kind, queuedTarget, ok := brain.NextIntention(); !ok || kind != IntentionAttack || queuedTarget != target {
		t.Fatalf("NextIntention() = (%v,%v,%v), want attack,target,true", kind, queuedTarget, ok)
	}

	strike.attackingNow = false
	brain.Think()
	if strike.target != target {
		t.Fatalf("attack target after Think = %v, want target", strike.target)
	}
}

func TestSummonAIThinkPreservesQueuedRetargetWhileCurrentAttackIsBusy(t *testing.T) {
	owner := actor(100)
	firstTarget := actor(200)
	secondTarget := actor(300)
	strike := &recordingAttack{canAttack: true}
	brain := NewSummon(owner, &summonMove{}, strike)

	if !brain.TryToAttack(firstTarget) {
		t.Fatal("TryToAttack(first) = false, want accepted attack")
	}
	if strike.target != firstTarget {
		t.Fatalf("attack target = %v, want first target", strike.target)
	}

	strike.attackingNow = true
	if !brain.TryToAttack(secondTarget) {
		t.Fatal("TryToAttack(second) = false, want queued retarget accepted while busy")
	}
	brain.Think()

	if kind, queuedTarget, ok := brain.NextIntention(); !ok || kind != IntentionAttack || queuedTarget != secondTarget {
		t.Fatalf("NextIntention() = (%v,%v,%v), want attack,second target,true", kind, queuedTarget, ok)
	}
}

// TestSummonThinkStopsMovementAndAttacksOnTheSameTick pins
// thinkAttackLocked's two-call shape (mirroring Attackable.thinkAttack): an
// accepted swing cancels the walk and starts the attack in the same tick,
// and logs nothing for either.
func TestSummonThinkStopsMovementAndAttacksOnTheSameTick(t *testing.T) {
	owner := actor(100)
	target := actor(200)
	move := &summonMove{}
	strike := &recordingAttack{canAttack: true}
	brain := NewSummon(owner, move, strike)
	var buf bytes.Buffer
	brain.SetLogger(zerolog.New(&buf))

	if !brain.TryToAttack(target) {
		t.Fatal("TryToAttack() = false, want accepted attack")
	}

	if move.stopCount != 1 {
		t.Fatalf("move.Stop calls = %d, want 1", move.stopCount)
	}
	if strike.doAttackCalls != 1 || strike.target != target {
		t.Fatalf("DoAttack calls = (%d, %v), want (1, target)", strike.doAttackCalls, strike.target)
	}
	if logged := buf.String(); logged != "" {
		t.Fatalf("logged = %q, want nothing logged on the accepted path", logged)
	}
}

// TestSummonAITryToAttackContinuesAgainstFakeDeadTarget is the regression
// test for the review finding that targetLostLocked treated AlikeDead()
// (fake death included) as target loss, dropping the attack intention the
// moment a target entered fake death. AbstractAI.isTargetLost
// (AbstractAI.java:586-594) and SummonAI's override (SummonAI.java:275-281)
// have no death check at all — only a nil or no-longer-known target is
// "lost".
func TestSummonAITryToAttackContinuesAgainstFakeDeadTarget(t *testing.T) {
	owner := actor(100)
	target := actor(200)
	target.alikeDead = true
	move := &summonMove{}
	strike := &recordingAttack{canAttack: true}
	brain := NewSummon(owner, move, strike)

	if !brain.TryToAttack(target) {
		t.Fatal("TryToAttack() = false against a fake-dead target, want the swing to proceed")
	}
	if strike.target != target {
		t.Fatalf("attack target = %v, want target despite fake death", strike.target)
	}
}

func TestSummonAITryToFollowStartsFriendlyFollow(t *testing.T) {
	owner := actor(100)
	target := actor(200)
	move := &summonMove{}
	brain := NewSummon(owner, move, &recordingAttack{})

	if !brain.TryToFollow(target) {
		t.Fatal("TryToFollow() = false, want accepted follow")
	}
	if move.friendlyTarget != target || move.friendlyRange != 70 {
		t.Fatalf("friendly follow = (%v, %d), want (%v, 70)", move.friendlyTarget, move.friendlyRange, target)
	}
	if got := brain.CurrentIntention(); got != IntentionFollow {
		t.Fatalf("CurrentIntention() = %v, want follow", got)
	}
}

func TestSummonAITryToCastExecutesImmediately(t *testing.T) {
	owner := actor(100)
	target := actor(200)
	cast := &recordingCast{canAttempt: true, canCast: true}
	brain := NewSummon(owner, &summonMove{}, &recordingAttack{})
	brain.SetCastController(cast)
	ref := skill.Ref{ID: 4139, Level: 8}

	if !brain.TryToCast(target, ref, false) {
		t.Fatal("TryToCast() = false, want accepted cast")
	}
	if cast.castedTarget != target || cast.castedRef != ref {
		t.Fatalf("cast = (%v,%v), want (%v,%v)", cast.castedTarget, cast.castedRef, target, ref)
	}
	if got := brain.CurrentIntention(); got != IntentionIdle {
		t.Fatalf("CurrentIntention() = %v, want idle once the cast is dispatched", got)
	}
}

func TestSummonAITryToCastAimsAtTheFinalTarget(t *testing.T) {
	owner := actor(100)
	clicked := actor(200)
	cast := &recordingCast{canAttempt: true, canCast: true, final: owner}
	brain := NewSummon(owner, &summonMove{}, &recordingAttack{})
	brain.SetCastController(cast)

	if !brain.TryToCast(clicked, skill.Ref{ID: 4139, Level: 8}, false) {
		t.Fatal("TryToCast() = false, want accepted cast")
	}
	if cast.castedTarget != owner {
		t.Fatalf("cast target = %v, want the final target %v", cast.castedTarget, owner)
	}
}

func TestSummonAITryToCastWithoutFinalTargetIsDropped(t *testing.T) {
	cast := &recordingCast{canAttempt: true, canCast: true, noFinal: true}
	brain := NewSummon(actor(100), &summonMove{}, &recordingAttack{})
	brain.SetCastController(cast)

	if brain.TryToCast(actor(200), skill.Ref{ID: 4139, Level: 8}, false) {
		t.Fatal("TryToCast() = true with no final target, want the request dropped")
	}
	if cast.castCalled || brain.CurrentIntention() != IntentionIdle {
		t.Fatalf("dropped request cast = %v, intention = %v; want no cast, idle", cast.castCalled, brain.CurrentIntention())
	}
}

func TestSummonAITryToCastApproachesBeforeCasting(t *testing.T) {
	owner := actor(100)
	target := actor(200)
	move := &summonMove{recordingMove: recordingMove{followStarted: true}}
	cast := &recordingCast{canAttempt: true, canCast: true, castRange: 400}
	brain := NewSummon(owner, move, &recordingAttack{})
	brain.SetCastController(cast)

	brain.TryToCast(target, skill.Ref{ID: 4139, Level: 8}, false)
	if cast.castCalled {
		t.Fatal("Cast() called while summon is closing distance")
	}
	if move.followTarget != target || move.followRange != 400 {
		t.Fatalf("offensive follow = (%v, %d), want (%v, 400)", move.followTarget, move.followRange, target)
	}
}

func TestSummonAITryToCastStopsFacesAndReportsFinalFailure(t *testing.T) {
	owner := actor(100)
	target := actor(200)
	move := &summonMove{}
	cast := &recordingCast{canAttempt: true, canCast: false, stopsMove: true}
	brain := NewSummon(owner, move, &recordingAttack{})
	brain.SetCastController(cast)

	brain.TryToCast(target, skill.Ref{ID: 4139, Level: 8}, false)
	if move.stopCount != 1 {
		t.Fatalf("Stop calls = %d, want 1", move.stopCount)
	}
	if owner.headingTarget != target {
		t.Fatalf("heading target = %v, want target", owner.headingTarget)
	}
	if owner.moveToPawnCalls != 1 || owner.moveToPawnTo != target {
		t.Fatalf("BroadcastMoveToPawn calls = (%d, %v), want (1, target)", owner.moveToPawnCalls, owner.moveToPawnTo)
	}
}

func TestSummonAITryToCastDropsBusyCastIntention(t *testing.T) {
	owner := actor(100)
	target := actor(200)
	strike := &recordingAttack{attackingNow: true}
	cast := &recordingCast{canAttempt: true, canCast: true}
	brain := NewSummon(owner, &summonMove{}, strike)
	brain.SetCastController(cast)

	if !brain.TryToCast(target, skill.Ref{ID: 4139, Level: 8}, false) {
		t.Fatal("TryToCast() = false, want queued cast intention")
	}
	strike.attackingNow = false
	cast.disabled = true
	brain.Think()
	if got := brain.CurrentIntention(); got != IntentionIdle {
		t.Fatalf("CurrentIntention() = %v, want idle while cast controller is busy", got)
	}
}

func TestSummonAITryToAttackDoesNotWaitForDisabledSkills(t *testing.T) {
	owner := actor(100)
	target := actor(200)
	strike := &recordingAttack{canAttack: true}
	brain := NewSummon(owner, &summonMove{}, strike)
	brain.SetCastController(&recordingCast{disabled: true})

	if !brain.TryToAttack(target) {
		t.Fatal("TryToAttack() = false, want attack accepted while skills are disabled")
	}
	if strike.target != target {
		t.Fatalf("attack target = %v, want target without a queued wait", strike.target)
	}
}

func TestSummonAITryToCastQueuesWhileBusyAndExecutesOnThink(t *testing.T) {
	owner := actor(100)
	target := actor(200)
	strike := &recordingAttack{canAttack: true, attackingNow: true}
	cast := &recordingCast{canAttempt: true, canCast: true}
	brain := NewSummon(owner, &summonMove{}, strike)
	brain.SetCastController(cast)
	ref := skill.Ref{ID: 4139, Level: 8}

	if !brain.TryToCast(target, ref, false) {
		t.Fatal("TryToCast() = false, want queued cast accepted while attacking")
	}
	if cast.castedTarget != nil {
		t.Fatalf("cast target = %v while busy, want no cast dispatched yet", cast.castedTarget)
	}
	if kind, queuedTarget, ok := brain.NextIntention(); !ok || kind != IntentionCast || queuedTarget != target {
		t.Fatalf("NextIntention() = (%v,%v,%v), want cast,target,true", kind, queuedTarget, ok)
	}

	strike.attackingNow = false
	brain.Think()
	if cast.castedTarget != target {
		t.Fatalf("cast target after Think = %v, want target", cast.castedTarget)
	}
}

func TestSummonAITryToCastRejectsWithoutCastController(t *testing.T) {
	brain := NewSummon(actor(100), &summonMove{}, &recordingAttack{})

	if brain.TryToCast(actor(200), skill.Ref{ID: 4139, Level: 8}, false) {
		t.Fatal("TryToCast() = true with no CastController attached, want false")
	}
}

func TestSummonAITryToCastRejectsWhenCanAttemptFails(t *testing.T) {
	target := actor(200)
	cast := &recordingCast{canAttempt: false, canCast: true}
	brain := NewSummon(actor(100), &summonMove{}, &recordingAttack{})
	brain.SetCastController(cast)

	if brain.TryToCast(target, skill.Ref{ID: 4139, Level: 8}, false) {
		t.Fatal("TryToCast() = true when CanAttempt rejects the skill, want false")
	}
	if cast.castedTarget != nil {
		t.Fatalf("cast target = %v, want no cast dispatched", cast.castedTarget)
	}
}

func TestSummonAITryToIdleStopsMovement(t *testing.T) {
	move := &summonMove{}
	brain := NewSummon(actor(100), move, &recordingAttack{})

	brain.TryToIdle()

	if move.stopCount != 1 {
		t.Fatalf("Stop calls = %d, want 1", move.stopCount)
	}
	if got := brain.CurrentIntention(); got != IntentionIdle {
		t.Fatalf("CurrentIntention() = %v, want idle", got)
	}
}

// TestSummonAIRecheckOffensiveFollowReissuesOnMovingTarget is the coverage
// for #1960: CreatureMove.java's 500 ms ATTACK_FOLLOW_INTERVAL
// (CreatureMove.java:41,556-561) re-evaluates an in-flight offensive follow
// on its own schedule, independent of the shared 1 s AI think tick. Each
// recheckOffensiveFollow call simulates one of those 500 ms ticks; a
// moving/out-of-range target keeps reissuing the follow request without
// waiting for Think.
func TestSummonAIRecheckOffensiveFollowReissuesOnMovingTarget(t *testing.T) {
	owner := actor(100)
	target := actor(200)
	move := &summonMove{recordingMove: recordingMove{followStarted: true}}
	brain := NewSummon(owner, move, &recordingAttack{canAttack: true})

	if !brain.TryToAttack(target) {
		t.Fatal("TryToAttack() = false, want accepted attack")
	}
	if move.followCalls != 1 {
		t.Fatalf("followCalls after TryToAttack = %d, want 1", move.followCalls)
	}

	brain.recheckOffensiveFollow()
	brain.recheckOffensiveFollow()

	if move.followCalls != 3 {
		t.Fatalf("followCalls after two 500 ms rechecks = %d, want 3 (1 initial + 2 rechecks)", move.followCalls)
	}
	if got := brain.CurrentIntention(); got != IntentionAttack {
		t.Fatalf("CurrentIntention() = %v, want attack still in flight", got)
	}
}

// TestSummonAIRecheckOffensiveFollowDoesNotReattackInRange is the
// regression test for the review finding that recheckOffensiveFollow
// called the full thinkAttackLocked, which falls through to
// attack.DoAttack whenever the summon is already in range (following ==
// false) and not busy. Java's ATTACK_FOLLOW_INTERVAL task
// (offensiveFollowTask, CreatureMove.java:563-584) only manages movement
// and never initiates an attack, so a summon whose weapon clears its
// cooldown inside 500 ms must not get a second DoAttack from this ticker.
func TestSummonAIRecheckOffensiveFollowDoesNotReattackInRange(t *testing.T) {
	owner := actor(100)
	target := actor(200)
	move := &summonMove{} // followStarted defaults to false: already in range.
	strike := &recordingAttack{canAttack: true}
	brain := NewSummon(owner, move, strike)

	if !brain.TryToAttack(target) {
		t.Fatal("TryToAttack() = false, want accepted attack")
	}
	if strike.doAttackCalls != 1 {
		t.Fatalf("doAttackCalls after TryToAttack = %d, want 1", strike.doAttackCalls)
	}

	brain.recheckOffensiveFollow()

	if strike.doAttackCalls != 1 {
		t.Fatalf("doAttackCalls after 500 ms recheck = %d, want 1 (recheck must not re-attack)", strike.doAttackCalls)
	}
}

// TestSummonAIRecheckOffensiveFollowDoesNotRecastInRange is the cast-path
// counterpart of TestSummonAIRecheckOffensiveFollowDoesNotReattackInRange:
// the 500 ms recheck must not fall through to cast.Cast either.
func TestSummonAIRecheckOffensiveFollowDoesNotRecastInRange(t *testing.T) {
	owner := actor(100)
	target := actor(200)
	// Starts out of range (chasing), so TryToCast queues the approach
	// instead of casting immediately, matching thinkCastLocked's
	// following==true early return and leaving the cast intention active.
	move := &summonMove{recordingMove: recordingMove{followStarted: true}}
	cast := &recordingCast{canAttempt: true, canCast: true}
	brain := NewSummon(owner, move, &recordingAttack{})
	brain.SetCastController(cast)
	ref := skill.Ref{ID: 4139, Level: 8}

	if !brain.TryToCast(target, ref, false) {
		t.Fatal("TryToCast() = false, want accepted cast approach")
	}
	if cast.castCalls != 0 {
		t.Fatalf("castCalls after TryToCast approach = %d, want 0 (still chasing)", cast.castCalls)
	}
	if got := brain.CurrentIntention(); got != IntentionCast {
		t.Fatalf("CurrentIntention() after TryToCast approach = %v, want cast still in flight", got)
	}

	// Target now sits in range: the 500 ms recheck (recheckOffensiveFollow)
	// must only re-evaluate the follow, not fall through to cast.Cast —
	// that execution belongs exclusively to the shared 1 s Think cadence.
	move.followStarted = false
	brain.recheckOffensiveFollow()

	if cast.castCalls != 0 {
		t.Fatalf("castCalls after 500 ms recheck = %d, want 0 (recheck must not cast)", cast.castCalls)
	}
	if got := brain.CurrentIntention(); got != IntentionCast {
		t.Fatalf("CurrentIntention() after recheck = %v, want cast still pending for Think", got)
	}
}

// TestSummonAIRecheckOffensiveFollowIgnoresFriendlyFollow proves the 500 ms
// offensive-follow recheck is a no-op outside an attack/cast intention, so
// friendly (owner) follow stays on the shared 1 s AI think cadence.
func TestSummonAIRecheckOffensiveFollowIgnoresFriendlyFollow(t *testing.T) {
	owner := actor(100)
	target := actor(200)
	move := &summonMove{}
	brain := NewSummon(owner, move, &recordingAttack{})

	if !brain.TryToFollow(target) {
		t.Fatal("TryToFollow() = false, want accepted follow")
	}

	brain.recheckOffensiveFollow()

	if move.followCalls != 0 {
		t.Fatalf("followCalls after recheck during friendly follow = %d, want 0", move.followCalls)
	}
}

// TestSummonAITargetLostDuringOffensiveFollowRecheckCancelsStaleMove is the
// known-list-loss coverage for #1960: SummonMove.java:48-53's follow-task
// branch forces the idle path's move.stop() when a followed target drops
// out of the known list mid-chase, so a stale movement leg can't keep
// running toward a target the summon no longer knows about.
func TestSummonAITargetLostDuringOffensiveFollowRecheckCancelsStaleMove(t *testing.T) {
	owner := actor(100)
	target := actor(200)
	move := &summonMove{recordingMove: recordingMove{followStarted: true}}
	brain := NewSummon(owner, move, &recordingAttack{canAttack: true})

	if !brain.TryToAttack(target) {
		t.Fatal("TryToAttack() = false, want accepted attack")
	}
	if move.stopCount != 0 {
		t.Fatalf("stopCount after TryToAttack = %d, want 0", move.stopCount)
	}

	owner.known[target.id] = false
	brain.recheckOffensiveFollow()

	if move.stopCount != 1 {
		t.Fatalf("stopCount after target lost = %d, want 1 (stale move canceled)", move.stopCount)
	}
	if got := brain.CurrentIntention(); got != IntentionIdle {
		t.Fatalf("CurrentIntention() = %v, want idle after target lost", got)
	}
}

type summonMove struct {
	recordingMove
	friendlyTarget attackable.Combatant
	friendlyRange  int
}

func (m *summonMove) MaybeStartFriendlyFollow(target attackable.Combatant, offset int) (bool, error) {
	m.friendlyTarget = target
	m.friendlyRange = offset
	return true, nil
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

func (recordingMove) MoveToLocation(location.Location) (bool, error) { return false, nil }

func (*fakeActor) IdleFollowTarget() attackable.Combatant { return nil }

func (*fakeActor) ThinkFollow(attackable.Combatant, bool) bool { return false }

// TestSummonAIFinishedCastingAppliesTheFollowItself pins that a cast ending
// with nothing to resume leaves the summon already following: the idle is
// decided and applied in one critical section, so a Betray TryToAttack from
// the caster's queue that lands once FinishedCasting returns keeps its attack
// instead of being overwritten by a later follow from the owner's queue.
func TestSummonAIFinishedCastingAppliesTheFollowItself(t *testing.T) {
	self := actor(100)
	owner := actor(1)
	move := &summonMove{}
	brain := NewSummon(self, move, &recordingAttack{canAttack: true})

	if !brain.FinishedCasting(owner) {
		t.Fatal("FinishedCasting() = false, want the summon sent idle")
	}
	if got := brain.CurrentIntention(); got != IntentionFollow {
		t.Fatalf("CurrentIntention() after FinishedCasting = %v, want follow already applied", got)
	}
	if move.friendlyTarget != owner {
		t.Fatalf("friendly follow target = %v, want the owner", move.friendlyTarget)
	}

	if !brain.TryToAttack(owner) {
		t.Fatal("Betray TryToAttack(owner) = false, want accepted")
	}
	if got := brain.CurrentIntention(); got != IntentionAttack {
		t.Fatalf("CurrentIntention() after Betray = %v, want attack", got)
	}
}

// TestSummonAIFinishedCastingWithoutFollowStandsStill pins the follow-off
// idle: nothing to resume and nobody to follow stops the summon in place.
func TestSummonAIFinishedCastingWithoutFollowStandsStill(t *testing.T) {
	move := &summonMove{}
	brain := NewSummon(actor(100), move, &recordingAttack{})

	if !brain.FinishedCasting(nil) {
		t.Fatal("FinishedCasting(nil) = false, want the summon sent idle")
	}
	if got := brain.CurrentIntention(); got != IntentionIdle {
		t.Fatalf("CurrentIntention() = %v, want idle", got)
	}
	if move.stopCount != 1 || move.friendlyTarget != nil {
		t.Fatalf("Stop calls = %d, follow target = %v; want 1 stop and no follow", move.stopCount, move.friendlyTarget)
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

// ---- summon step-aside ----
// stepAsideMove records every walk a summon AI starts.
type stepAsideMove struct {
	summonMove
	walks []location.Location
}

func (m *stepAsideMove) MoveToLocation(dest location.Location) (bool, error) {
	m.walks = append(m.walks, dest)
	return true, nil
}

// TestSummonAIStepAsideOnlyFromIdleOrFollow pins SummonMove.avoidAttack's
// intention gate: an idle or following summon walks to the spot and takes a
// MOVE_TO intention, while one attacking, casting or unable to act refuses
// and keeps its intention.
func TestSummonAIStepAsideOnlyFromIdleOrFollow(t *testing.T) {
	dest := location.Location{X: 1070, Y: 1000, Z: 0}
	ref := skill.Ref{ID: 4139, Level: 8}

	tests := []struct {
		name  string
		setup func(t *testing.T, brain *Summon, owner *fakeActor, strike *recordingAttack, cast *recordingCast)
		want  Intention
		walks bool
	}{
		{name: "idle", want: IntentionMoveTo, walks: true},
		{
			name: "follow",
			setup: func(t *testing.T, brain *Summon, _ *fakeActor, _ *recordingAttack, _ *recordingCast) {
				if !brain.TryToFollow(actor(1)) {
					t.Fatal("TryToFollow() = false, want accepted follow")
				}
			},
			want:  IntentionMoveTo,
			walks: true,
		},
		{
			name: "attack",
			setup: func(t *testing.T, brain *Summon, _ *fakeActor, strike *recordingAttack, _ *recordingCast) {
				if !brain.TryToAttack(actor(200)) {
					t.Fatal("TryToAttack() = false, want accepted attack")
				}
				// Between swings: only the intention holds the summon back.
				strike.attackingNow = false
			},
			want: IntentionAttack,
		},
		{
			name: "cast",
			setup: func(t *testing.T, brain *Summon, _ *fakeActor, _ *recordingAttack, cast *recordingCast) {
				// The summon is still closing distance, so the cast intention
				// stays current with no cast in flight.
				brain.TryToCast(actor(200), ref, false)
				if cast.castCalled {
					t.Fatal("Cast() called while closing distance, want the approach only")
				}
			},
			want: IntentionCast,
		},
		{
			name: "idle but denied AI action",
			setup: func(_ *testing.T, _ *Summon, owner *fakeActor, _ *recordingAttack, _ *recordingCast) {
				owner.denyAction = true
			},
			want: IntentionIdle,
		},
		{
			name: "idle mid-swing",
			setup: func(_ *testing.T, _ *Summon, _ *fakeActor, strike *recordingAttack, _ *recordingCast) {
				strike.attackingNow = true
			},
			want: IntentionIdle,
		},
		{
			name: "idle mid-cast",
			setup: func(_ *testing.T, _ *Summon, _ *fakeActor, _ *recordingAttack, cast *recordingCast) {
				cast.casting = true
			},
			want: IntentionIdle,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			owner := actor(100)
			move := &stepAsideMove{summonMove: summonMove{recordingMove: recordingMove{followStarted: true}}}
			strike := &recordingAttack{canAttack: true}
			cast := &recordingCast{canAttempt: true, canCast: true, castRange: 400}
			brain := NewSummon(owner, move, strike)
			brain.SetCastController(cast)
			if tc.setup != nil {
				tc.setup(t, brain, owner, strike, cast)
			}
			before := len(move.walks)

			got := brain.StepAside(dest)
			if got != tc.walks {
				t.Fatalf("StepAside() = %v, want %v", got, tc.walks)
			}
			if kind := brain.CurrentIntention(); kind != tc.want {
				t.Fatalf("CurrentIntention() = %v, want %v", kind, tc.want)
			}
			walks := move.walks[before:]
			if tc.walks {
				if len(walks) != 1 || walks[0] != dest {
					t.Fatalf("walks = %v, want [%v]", walks, dest)
				}
			} else if len(walks) != 0 {
				t.Fatalf("walks = %v, want none", walks)
			}
		})
	}
}

// ---- playable attack gate ----

// gateFake stands in for a playable on either side of the playable attack
// gate: a player, or a summon when owner is set. It also serves as the
// PlayerAttackActor and SummonActor driving Start and TryToAttack. The real
// player.Character and summon.Actor reach these values only through a live
// world (effects, zones, cursed weapons), and cursed weapons do not exist
// yet (#225), so the gate's branches are pinned here per
// docs/agents/test-strategy.md.
type gateFake struct {
	attackabletest.Combatant
	world.Presence
	id       int32
	kind     modelactor.Kind
	level    int
	karma    int
	blessed  bool
	cursed   bool
	pvp      bool
	owner    *gateFake
	casting  bool
	denied   bool
	betrayed bool
	refusals int
}

func (g *gateFake) ObjectID() int32          { return g.id }
func (g *gateFake) Betrayed() bool           { return g.betrayed }
func (g *gateFake) Kind() modelactor.Kind    { return g.kind }
func (g *gateFake) Level() int               { return g.level }
func (g *gateFake) Karma() int               { return g.karma }
func (g *gateFake) ProtectionBlessing() bool { return g.blessed }
func (g *gateFake) CursedWeaponEquipped() bool {
	return g.cursed
}
func (g *gateFake) InPvPZone() bool                          { return g.pvp }
func (g *gateFake) CastingNow() bool                         { return g.casting }
func (g *gateFake) DenyAIAction() bool                       { return g.denied }
func (g *gateFake) PhysicalAttackRange() int                 { return 40 }
func (g *gateFake) Standing() bool                           { return true }
func (g *gateFake) SetHeadingTo(attackable.Combatant)        {}
func (g *gateFake) BroadcastMoveToPawn(attackable.Combatant) {}
func (g *gateFake) RefuseAttackTarget()                      { g.refusals++ }
func (g *gateFake) Owner() (attackable.Combatant, bool) {
	if g.owner == nil {
		return nil, false
	}
	return g.owner, true
}

func gatePlayerFake(id int32, level, karma int) *gateFake {
	return &gateFake{id: id, kind: modelactor.KindPlayer, level: level, karma: karma}
}

func gateSummonFake(id int32, owner *gateFake) *gateFake {
	return &gateFake{id: id, kind: modelactor.KindSummon, level: 1, owner: owner}
}

func TestRefusesPlayableTarget(t *testing.T) {
	blessed := func(g *gateFake) *gateFake { g.blessed = true; return g }
	cursed := func(g *gateFake) *gateFake { g.cursed = true; return g }
	inPvP := func(g *gateFake) *gateFake { g.pvp = true; return g }
	npc := func(g *gateFake) *gateFake { g.kind = modelactor.KindNPC; return g }

	tests := []struct {
		name     string
		attacker attackable.Combatant
		target   attackable.Combatant
		want     bool
	}{
		{"karma attacker 10 levels above a blessed target", gatePlayerFake(1, 30, 500), blessed(gatePlayerFake(2, 20, 0)), true},
		{"karma attacker 9 levels above a blessed target", gatePlayerFake(1, 29, 500), blessed(gatePlayerFake(2, 20, 0)), false},
		{"karma-free attacker far above a blessed target", gatePlayerFake(1, 40, 0), blessed(gatePlayerFake(2, 20, 0)), false},
		{"blessed target inside a PvP zone, attacker outside", gatePlayerFake(1, 30, 500), inPvP(blessed(gatePlayerFake(2, 20, 0))), false},
		{"attacker inside a PvP zone, blessed target outside", inPvP(gatePlayerFake(1, 30, 500)), blessed(gatePlayerFake(2, 20, 0)), true},
		{"blessed attacker, karma target 10 levels above", blessed(gatePlayerFake(1, 20, 0)), gatePlayerFake(2, 30, 500), true},
		{"blessed attacker, karma target 9 levels above", blessed(gatePlayerFake(1, 20, 0)), gatePlayerFake(2, 29, 500), false},
		{"blessed attacker, karma-free target far above", blessed(gatePlayerFake(1, 20, 0)), gatePlayerFake(2, 40, 0), false},
		{"blessed attacker, karma target above inside a PvP zone", blessed(gatePlayerFake(1, 20, 0)), inPvP(gatePlayerFake(2, 30, 500)), false},
		{"level 20 attacker, cursed-weapon target", gatePlayerFake(1, 20, 0), cursed(gatePlayerFake(2, 40, 0)), true},
		{"level 21 attacker, cursed-weapon target", gatePlayerFake(1, 21, 0), cursed(gatePlayerFake(2, 40, 0)), false},
		{"cursed-weapon attacker, level 20 target", cursed(gatePlayerFake(1, 40, 0)), gatePlayerFake(2, 20, 0), true},
		{"cursed-weapon attacker, level 21 target", cursed(gatePlayerFake(1, 40, 0)), gatePlayerFake(2, 21, 0), false},
		{"cursed-weapon attacker, level 20 target inside a PvP zone", cursed(gatePlayerFake(1, 40, 0)), inPvP(gatePlayerFake(2, 20, 0)), true},
		{"non-playable target", cursed(gatePlayerFake(1, 40, 500)), npc(blessed(gatePlayerFake(2, 1, 0))), false},
		{"summon of a blessed player", gatePlayerFake(1, 30, 500), gateSummonFake(3, blessed(gatePlayerFake(2, 20, 0))), true},
		{"summon of a blessed player inside a PvP zone", gatePlayerFake(1, 30, 500), inPvP(gateSummonFake(3, blessed(gatePlayerFake(2, 20, 0)))), false},
		{"summon of a blessed player whose owner alone is in a PvP zone", gatePlayerFake(1, 30, 500), gateSummonFake(3, inPvP(blessed(gatePlayerFake(2, 20, 0)))), true},
		{"summon of a karma player at a blessed target", gateSummonFake(3, gatePlayerFake(1, 30, 500)), blessed(gatePlayerFake(2, 20, 0)), true},
		{"attacker with no acting player", actor(1), blessed(gatePlayerFake(2, 1, 0)), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := refusesPlayableTarget(tc.attacker, tc.target); got != tc.want {
				t.Fatalf("refusesPlayableTarget() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPlayerAttackRefusedTargetKeepsCurrentTarget(t *testing.T) {
	pk := gatePlayerFake(1, 30, 500)
	prev := gatePlayerFake(3, 30, 0)
	blessed := &gateFake{id: 2, kind: modelactor.KindPlayer, level: 10, blessed: true}
	strike := &recordingAttack{canAttack: true}
	brain := NewPlayerAttack(pk, &recordingMove{}, strike)

	if !brain.Start(prev, false) {
		t.Fatal("Start(prev) = false, want accepted")
	}
	strike.attackingNow = false
	if brain.Start(blessed, false) {
		t.Fatal("Start(blessed) = true, want refused")
	}
	if pk.refusals != 1 {
		t.Fatalf("refusals = %d, want 1", pk.refusals)
	}
	if got := brain.Target(); got != prev {
		t.Fatalf("Target() after refusal = %v, want the previous target", got)
	}
	if strike.doAttackCalls != 1 || strike.target != prev {
		t.Fatalf("swings = %d at %v, want only the one at the previous target", strike.doAttackCalls, strike.target)
	}

	if !brain.RefuseTarget(blessed) || pk.refusals != 2 {
		t.Fatalf("RefuseTarget(blessed) refusals = %d, want refused and reported", pk.refusals)
	}
	if brain.RefuseTarget(prev) || pk.refusals != 2 {
		t.Fatalf("RefuseTarget(prev) refusals = %d, want accepted silently", pk.refusals)
	}
	if got := brain.Target(); got != prev {
		t.Fatalf("Target() after RefuseTarget = %v, want the previous target", got)
	}
}

func TestPlayerAttackSkipsGateWhileDeniedOrBusy(t *testing.T) {
	for _, tc := range []struct {
		name string
		set  func(pk *gateFake, strike *recordingAttack)
	}{
		{"denied", func(pk *gateFake, _ *recordingAttack) { pk.denied = true }},
		{"casting", func(pk *gateFake, _ *recordingAttack) { pk.casting = true }},
		{"attacking", func(_ *gateFake, strike *recordingAttack) { strike.attackingNow = true }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pk := gatePlayerFake(1, 30, 500)
			blessed := &gateFake{id: 2, kind: modelactor.KindPlayer, level: 10, blessed: true}
			strike := &recordingAttack{canAttack: true}
			tc.set(pk, strike)
			brain := NewPlayerAttack(pk, &recordingMove{}, strike)

			if brain.RefuseTarget(blessed) {
				t.Fatal("RefuseTarget() = true, want the gate skipped")
			}
			brain.Start(blessed, false)
			if pk.refusals != 0 {
				t.Fatalf("refusals = %d, want 0 while %s", pk.refusals, tc.name)
			}
			if strike.doAttackCalls != 0 {
				t.Fatalf("swings = %d, want none while %s", strike.doAttackCalls, tc.name)
			}
		})
	}
}

func TestSummonAIRefusedTargetKeepsCurrentIntention(t *testing.T) {
	owner := gatePlayerFake(1, 30, 500)
	pet := gateSummonFake(3, owner)
	blessed := &gateFake{id: 2, kind: modelactor.KindPlayer, level: 10, blessed: true}
	strike := &recordingAttack{canAttack: true}
	brain := NewSummon(pet, &summonMove{}, strike)

	if !brain.TryToFollow(owner) {
		t.Fatal("TryToFollow(owner) = false, want accepted")
	}
	if brain.TryToAttack(blessed) {
		t.Fatal("TryToAttack(blessed) = true, want refused")
	}
	if pet.refusals != 1 {
		t.Fatalf("refusals = %d, want 1", pet.refusals)
	}
	if got := brain.CurrentIntention(); got != IntentionFollow {
		t.Fatalf("CurrentIntention() after refusal = %v, want follow", got)
	}
	if _, _, queued := brain.NextIntention(); queued {
		t.Fatal("refused attack was queued as the next intention")
	}
	if strike.doAttackCalls != 0 {
		t.Fatalf("swings = %d, want none", strike.doAttackCalls)
	}
}

func TestSummonAISkipsGateWhileDeniedOrBusy(t *testing.T) {
	for _, tc := range []struct {
		name   string
		set    func(pet *gateFake, strike *recordingAttack)
		queued bool
	}{
		{"denied", func(pet *gateFake, _ *recordingAttack) { pet.denied = true }, false},
		{"attacking", func(_ *gateFake, strike *recordingAttack) { strike.attackingNow = true }, true},
		{"bow cooling down", func(_ *gateFake, strike *recordingAttack) { strike.bowCooling = true }, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pet := gateSummonFake(3, gatePlayerFake(1, 30, 500))
			blessed := &gateFake{id: 2, kind: modelactor.KindPlayer, level: 10, blessed: true}
			strike := &recordingAttack{canAttack: true}
			tc.set(pet, strike)
			brain := NewSummon(pet, &summonMove{}, strike)

			brain.TryToAttack(blessed)
			if pet.refusals != 0 {
				t.Fatalf("refusals = %d, want 0 while %s", pet.refusals, tc.name)
			}
			kind, next, ok := brain.NextIntention()
			if tc.queued && (!ok || kind != IntentionAttack || next != blessed) {
				t.Fatalf("NextIntention() = (%v,%v,%v), want the attack queued", kind, next, ok)
			}
			if strike.doAttackCalls != 0 {
				t.Fatalf("swings = %d, want none while %s", strike.doAttackCalls, tc.name)
			}
		})
	}
}

// castingAfterAttack returns a summon AI that was attacking target when its
// owner commanded a cast on it, so the cast replaced the attack.
func castingAfterAttack(t *testing.T) (*Summon, *fakeActor, *recordingAttack, *recordingCast) {
	t.Helper()
	self, target := actor(100), actor(200)
	self.known[target.ObjectID()] = true
	strike := &recordingAttack{canAttack: true}
	cast := &recordingCast{canAttempt: true, canCast: true}
	brain := NewSummon(self, &summonMove{}, strike)
	brain.SetCastController(cast)
	if !brain.TryToAttack(target) || strike.doAttackCalls != 1 {
		t.Fatalf("TryToAttack() did not swing (DoAttack calls %d)", strike.doAttackCalls)
	}
	if !brain.TryToCast(target, skill.Ref{ID: 4139, Level: 1}, false) || cast.castCalls != 1 {
		t.Fatal("TryToCast() did not cast")
	}
	cast.casting = true
	return brain, target, strike, cast
}

// TestSummonAICastStoppedResumesTheReplacedAttack pins a stopped cast's AI
// step: with nothing queued it resumes the attack the cast replaced and
// reports nothing idled.
func TestSummonAICastStoppedResumesTheReplacedAttack(t *testing.T) {
	brain, target, strike, cast := castingAfterAttack(t)
	cast.casting = false

	idled, handled := brain.CastStopped(actor(1))
	if !handled || idled {
		t.Fatalf("CastStopped() = (idled %v, handled %v), want (false, true)", idled, handled)
	}
	if got := brain.CurrentIntention(); got != IntentionAttack {
		t.Fatalf("CurrentIntention() = %v, want the attack resumed", got)
	}
	if strike.doAttackCalls != 2 || strike.target != target {
		t.Fatalf("DoAttack calls = %d on %v, want a second swing on the target", strike.doAttackCalls, strike.target)
	}
}

// TestSummonAICastStoppedDropsTheQueuedCast pins that a cast queued behind
// a stopped cast is dropped, not started: the summon goes idle, following
// its owner, rather than resuming the attack the first cast replaced or
// firing the queued skill through the interrupt. Without an owner to follow
// it stands still.
func TestSummonAICastStoppedDropsTheQueuedCast(t *testing.T) {
	for _, tc := range []struct {
		name   string
		follow attackable.Combatant
		want   Intention
	}{
		{name: "follows owner", follow: actor(1), want: IntentionFollow},
		{name: "no owner", follow: nil, want: IntentionIdle},
	} {
		t.Run(tc.name, func(t *testing.T) {
			brain, target, strike, cast := castingAfterAttack(t)
			if brain.TryToCast(target, skill.Ref{ID: 4140, Level: 1}, false); cast.castCalls != 1 {
				t.Fatalf("TryToCast() while casting started a cast (calls %d), want it queued", cast.castCalls)
			}
			cast.casting = false

			idled, handled := brain.CastStopped(tc.follow)
			if !handled || !idled {
				t.Fatalf("CastStopped() = (idled %v, handled %v), want (true, true)", idled, handled)
			}
			if cast.castCalls != 1 {
				t.Fatalf("Cast calls = %d, want the queued cast dropped", cast.castCalls)
			}
			if strike.doAttackCalls != 1 {
				t.Fatalf("DoAttack calls = %d, want no resumed swing", strike.doAttackCalls)
			}
			if got := brain.CurrentIntention(); got != tc.want {
				t.Fatalf("CurrentIntention() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestSummonAIFinishedCastingStartsTheQueuedCast pins the other side: a
// cast that completes starts the cast queued behind it.
func TestSummonAIFinishedCastingStartsTheQueuedCast(t *testing.T) {
	brain, target, _, cast := castingAfterAttack(t)
	if brain.TryToCast(target, skill.Ref{ID: 4140, Level: 1}, false); cast.castCalls != 1 {
		t.Fatalf("TryToCast() while casting started a cast (calls %d), want it queued", cast.castCalls)
	}
	cast.casting = false

	if idled := brain.FinishedCasting(actor(1)); idled {
		t.Fatal("FinishedCasting() idled, want the queued cast started")
	}
	if cast.castCalls != 2 || cast.castedRef.ID != 4140 {
		t.Fatalf("Cast calls = %d (last %v), want the queued skill 4140 cast", cast.castCalls, cast.castedRef)
	}
}

// TestSummonAICastStoppedWithoutAttackFollowsOwner pins a stopped cast with
// no attack to resume: the summon goes back to following its owner.
func TestSummonAICastStoppedWithoutAttackFollowsOwner(t *testing.T) {
	self, target, owner := actor(100), actor(200), actor(1)
	self.known[target.ObjectID()] = true
	move := &summonMove{}
	cast := &recordingCast{canAttempt: true, canCast: true}
	brain := NewSummon(self, move, &recordingAttack{})
	brain.SetCastController(cast)
	if !brain.TryToCast(target, skill.Ref{ID: 4139, Level: 1}, false) {
		t.Fatal("TryToCast() did not cast")
	}

	idled, handled := brain.CastStopped(owner)
	if !handled || !idled {
		t.Fatalf("CastStopped() = (idled %v, handled %v), want (true, true)", idled, handled)
	}
	if got := brain.CurrentIntention(); got != IntentionFollow || move.friendlyTarget != owner {
		t.Fatalf("CurrentIntention() = %v following %v, want follow on the owner", got, move.friendlyTarget)
	}
}

// TestSummonAIAbortAllLeavesTheStoppedCastAlone pins that a cast AbortAll
// stops moves nothing on: the cast end it reports is left unhandled, so no
// attack resumes before AbortAll's caller settles the summon, while a cast
// stopped outside AbortAll afterwards is handled again.
func TestSummonAIAbortAllLeavesTheStoppedCastAlone(t *testing.T) {
	brain, _, strike, cast := castingAfterAttack(t)
	var handled, called bool
	cast.onStop = func() {
		called = true
		_, handled = brain.CastStopped(actor(1))
	}

	brain.AbortAll()
	if !called || handled {
		t.Fatalf("cast end during AbortAll: reported %v, handled %v; want reported and unhandled", called, handled)
	}
	if strike.doAttackCalls != 1 {
		t.Fatalf("DoAttack calls = %d after AbortAll, want no resumed swing", strike.doAttackCalls)
	}
	if got := brain.CurrentIntention(); got != IntentionIdle {
		t.Fatalf("CurrentIntention() = %v after AbortAll, want the cast's idle left as is", got)
	}

	cast.onStop = nil
	if _, handled := brain.CastStopped(actor(1)); !handled {
		t.Fatal("CastStopped() after AbortAll returned unhandled, want the abort guard released")
	}
}

// ---- keep attacking after a swing ----

func TestCanKeepAttacking(t *testing.T) {
	inPvP := func(g *gateFake) *gateFake { g.pvp = true; return g }
	betrayedSummon := func(g *gateFake) *gateFake { g.betrayed = true; return g }
	npc := &gateFake{id: 9, kind: modelactor.KindNPC}

	tests := []struct {
		name     string
		attacker attackable.Combatant
		target   attackable.Combatant
		want     bool
	}{
		{"no target", gatePlayerFake(1, 40, 0), nil, false},
		{"non-playable target", gatePlayerFake(1, 40, 0), npc, true},
		{"unflagged player", gatePlayerFake(1, 40, 0), gatePlayerFake(2, 40, 0), false},
		{"karma player", gatePlayerFake(1, 40, 0), gatePlayerFake(2, 40, 500), true},
		{"summon of a karma player", gatePlayerFake(1, 40, 0), gateSummonFake(3, gatePlayerFake(2, 40, 500)), true},
		{"summon of an unflagged player", gatePlayerFake(1, 40, 0), gateSummonFake(3, gatePlayerFake(2, 40, 0)), false},
		{"both inside a PvP zone", inPvP(gatePlayerFake(1, 40, 0)), inPvP(gatePlayerFake(2, 40, 0)), true},
		{"only the target inside a PvP zone", gatePlayerFake(1, 40, 0), inPvP(gatePlayerFake(2, 40, 0)), false},
		{"only the attacker inside a PvP zone", inPvP(gatePlayerFake(1, 40, 0)), gatePlayerFake(2, 40, 0), false},
		{"summon in a PvP zone at a player in one", inPvP(gateSummonFake(3, gatePlayerFake(1, 40, 0))), inPvP(gatePlayerFake(2, 40, 0)), true},
		{"betrayed summon at its owner", betrayedSummon(gateSummonFake(3, gatePlayerFake(1, 40, 0))), gatePlayerFake(1, 40, 0), true},
		{"loyal summon at an unflagged player", gateSummonFake(3, gatePlayerFake(1, 40, 0)), gatePlayerFake(2, 40, 0), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := canKeepAttacking(tc.attacker, tc.target); got != tc.want {
				t.Fatalf("canKeepAttacking() = %v, want %v", got, tc.want)
			}
		})
	}
}

// A swing that ends with nothing queued swings again only at a target the
// player can keep attacking; any other goes idle silently.
func TestPlayerAttackFinishedAttackKeepsOnlyKeepableTargets(t *testing.T) {
	for _, tc := range []struct {
		name  string
		karma int
		swing bool
	}{
		{"unflagged player", 0, false},
		{"karma player", 500, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pc := gatePlayerFake(1, 40, 0)
			target := gatePlayerFake(2, 40, tc.karma)
			strike := &recordingAttack{canAttack: true}
			move := &recordingMove{}
			brain := NewPlayerAttack(pc, move, strike)
			if !brain.Start(target, false) {
				t.Fatal("Start() = false, want the first swing")
			}
			strike.attackingNow = false

			if brain.FinishedAttack() {
				t.Fatal("FinishedAttack() = true, want no ActionFailed")
			}
			wantSwings := 1
			if tc.swing {
				wantSwings = 2
			}
			if strike.doAttackCalls != wantSwings {
				t.Fatalf("swings = %d, want %d", strike.doAttackCalls, wantSwings)
			}
			if kept := brain.Target() != nil; kept != tc.swing {
				t.Fatalf("attack intention kept = %v, want %v", kept, tc.swing)
			}
		})
	}
}

// An attack requested again mid-swing is the next intention: it runs when
// the swing ends, keepable target or not.
func TestPlayerAttackFinishedAttackRunsTheQueuedAttack(t *testing.T) {
	pc := gatePlayerFake(1, 40, 0)
	target := gatePlayerFake(2, 40, 0)
	strike := &recordingAttack{canAttack: true}
	brain := NewPlayerAttack(pc, &recordingMove{}, strike)
	if !brain.Start(target, false) {
		t.Fatal("Start() = false, want the first swing")
	}
	brain.Start(target, false)
	strike.attackingNow = false

	brain.FinishedAttack()
	if strike.doAttackCalls != 2 {
		t.Fatalf("swings = %d, want the queued attack to swing again", strike.doAttackCalls)
	}
}

// A dead unflagged player goes idle silently; a dead karma player is thought
// once more and lost, answered ActionFailed.
func TestPlayerAttackFinishedAttackOnDeadTarget(t *testing.T) {
	for _, tc := range []struct {
		name         string
		karma        int
		actionFailed bool
	}{
		{"unflagged player", 0, false},
		{"karma player", 500, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pc := gatePlayerFake(1, 40, 0)
			target := &deadGateFake{gateFake: gatePlayerFake(2, 40, tc.karma)}
			strike := &recordingAttack{canAttack: true}
			brain := NewPlayerAttack(pc, &recordingMove{}, strike)
			if !brain.Start(target, false) {
				t.Fatal("Start() = false, want the first swing")
			}
			strike.attackingNow = false
			target.dead = true

			if got := brain.FinishedAttack(); got != tc.actionFailed {
				t.Fatalf("FinishedAttack() = %v, want %v", got, tc.actionFailed)
			}
			if brain.Target() != nil {
				t.Fatal("attack intention kept on a dead target")
			}
		})
	}
}

type deadGateFake struct {
	*gateFake
	dead bool
}

func (d *deadGateFake) AlikeDead() bool { return d.dead }
func (d *deadGateFake) Dead() bool      { return d.dead }

// A summon that cannot keep attacking its target goes idle once the swing
// ends: back to following its owner, or standing still with follow off.
func TestSummonAIFinishedAttackIdlesOnUnkeepableTarget(t *testing.T) {
	owner := gatePlayerFake(1, 40, 0)
	pet := gateSummonFake(3, owner)
	target := gatePlayerFake(2, 40, 0)
	strike := &recordingAttack{canAttack: true}
	move := &summonMove{}
	brain := NewSummon(pet, move, strike)
	if !brain.TryToAttack(target) {
		t.Fatal("TryToAttack() = false, want the first swing")
	}
	strike.attackingNow = false

	if !brain.FinishedAttack(owner) {
		t.Fatal("FinishedAttack() = false, want the summon sent idle")
	}
	if got := brain.CurrentIntention(); got != IntentionFollow {
		t.Fatalf("CurrentIntention() = %v, want follow", got)
	}
	if move.friendlyTarget != owner {
		t.Fatalf("friendly follow target = %v, want the owner", move.friendlyTarget)
	}
	if strike.doAttackCalls != 1 {
		t.Fatalf("swings = %d, want 1", strike.doAttackCalls)
	}

	still := NewSummon(gateSummonFake(4, owner), &summonMove{}, &recordingAttack{canAttack: true})
	still.TryToAttack(target)
	if !still.FinishedAttack(nil) || still.CurrentIntention() != IntentionIdle {
		t.Fatalf("follow-off FinishedAttack: intention = %v, want idle", still.CurrentIntention())
	}
}

// Against a keepable target, or with an intention queued, the swing's end
// carries on as before.
func TestSummonAIFinishedAttackKeepsKeepableTargets(t *testing.T) {
	owner := gatePlayerFake(1, 40, 0)
	pet := gateSummonFake(3, owner)
	strike := &recordingAttack{canAttack: true}
	brain := NewSummon(pet, &summonMove{}, strike)
	karma := gatePlayerFake(2, 40, 500)
	brain.TryToAttack(karma)
	strike.attackingNow = false

	if brain.FinishedAttack(owner) {
		t.Fatal("FinishedAttack() = true, want the attack kept")
	}
	if strike.doAttackCalls != 2 || brain.CurrentIntention() != IntentionAttack {
		t.Fatalf("swings = %d, intention = %v; want a second swing on the attack", strike.doAttackCalls, brain.CurrentIntention())
	}

	white := gatePlayerFake(5, 40, 0)
	strike.attackingNow = true
	brain.TryToAttack(white)
	strike.attackingNow = false
	if brain.FinishedAttack(owner) {
		t.Fatal("FinishedAttack() with a queued attack = true, want it run")
	}
	if strike.target != white {
		t.Fatalf("swing target = %v, want the queued attack's", strike.target)
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

// A shift-held attack never walks: on a target out of reach it goes idle,
// answered ActionFailed, with no follow and no swing. Within reach it swings
// as an unshifted attack does.
func TestPlayerAttackShiftHeldNeverWalks(t *testing.T) {
	pc := gatePlayerFake(1, 40, 0)
	target := gatePlayerFake(2, 40, 500)
	strike := &recordingAttack{canAttack: true}
	move := &recordingMove{outOfReach: true, followStarted: true}
	brain := NewPlayerAttack(pc, move, strike)

	if brain.Start(target, true) {
		t.Fatal("Start(shift) on a target out of reach = true, want ActionFailed")
	}
	if move.followCalls != 0 || move.holdCalls != 1 {
		t.Fatalf("follow calls = %d, hold calls = %d, want 0 and 1", move.followCalls, move.holdCalls)
	}
	if strike.doAttackCalls != 0 {
		t.Fatalf("swings = %d, want none", strike.doAttackCalls)
	}
	if brain.Target() != nil {
		t.Fatal("shift-held attack out of reach kept its intention, want idle")
	}
	if move.stopCount == 0 {
		t.Fatal("idle did not stop movement")
	}

	move.outOfReach = false
	if !brain.Start(target, true) {
		t.Fatal("Start(shift) within reach = false, want the swing")
	}
	if strike.doAttackCalls != 1 || move.followCalls != 0 {
		t.Fatalf("swings = %d, follow calls = %d, want 1 and 0", strike.doAttackCalls, move.followCalls)
	}

	// The target steps out of reach before the swing ends: the re-think
	// goes idle instead of chasing it.
	strike.attackingNow = false
	move.outOfReach = true
	if !brain.FinishedAttack() {
		t.Fatal("FinishedAttack() on a shift-held attack out of reach = false, want ActionFailed")
	}
	if move.followCalls != 0 || brain.Target() != nil {
		t.Fatalf("follow calls = %d, intention kept = %v, want no follow and idle", move.followCalls, brain.Target() != nil)
	}
}

// The attack a nextActionAttack cast hands on to holds the cast's shift.
func TestPlayerAttackAfterShiftCastNeverWalks(t *testing.T) {
	pc := gatePlayerFake(1, 40, 0)
	target := gatePlayerFake(2, 40, 500)
	strike := &recordingAttack{canAttack: true}
	move := &recordingMove{outOfReach: true, followStarted: true}
	brain := NewPlayerAttack(pc, move, strike)

	if !brain.AttackAfterCast(target, true) {
		t.Fatal("AttackAfterCast(shift) out of reach = no ActionFailed, want ActionFailed")
	}
	if move.followCalls != 0 || strike.doAttackCalls != 0 || brain.Target() != nil {
		t.Fatalf("follow calls = %d, swings = %d, intention kept = %v, want none and idle", move.followCalls, strike.doAttackCalls, brain.Target() != nil)
	}

	if brain.AttackAfterCast(target, false) {
		t.Fatal("AttackAfterCast(no shift) out of reach = ActionFailed, want the walk")
	}
	if move.followCalls != 1 {
		t.Fatalf("follow calls = %d, want the walk toward the target", move.followCalls)
	}
}
