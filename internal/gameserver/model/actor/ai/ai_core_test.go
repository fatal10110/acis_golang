package ai

import (
	"bytes"
	"errors"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/rs/zerolog"
)

// ---- from attackable_attack_test.go ----
func addAttackHate(ai *Attackable, attacker attackable.Combatant, damage, hate float64) {
	ai.AddDamageHate(attacker, damage, hate)
	ai.AddAttackDesire(attacker, hate)
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
	ai.Think()

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

// TestAttackableThinkJoinsStopAndAttackBroadcastErrors is the regression
// test for the review finding that thinkAttack's `if stopErr != nil {
// return stopErr }` masked attackErr whenever both move.Stop's and
// attack.DoAttack's broadcasts failed in the same tick: only one of the two
// failures ever reached the log. Both errors must now surface.
func TestAttackableThinkJoinsStopAndAttackBroadcastErrors(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	owner.known = map[int32]bool{target.ObjectID(): true}
	stopErr := errors.New("stop broadcast failed")
	attackErr := errors.New("attack broadcast failed")
	move := &recordingMove{stopErr: stopErr}
	strike := &recordingAttack{canAttack: true, doAttackErr: attackErr}
	ai := NewAttackable(owner, move, strike)

	addAttackHate(ai, target, 0, 10)
	err := ai.Think()

	if !errors.Is(err, stopErr) {
		t.Fatalf("Think() error = %v, want it to wrap stopErr (%v)", err, stopErr)
	}
	if !errors.Is(err, attackErr) {
		t.Fatalf("Think() error = %v, want it to wrap attackErr (%v)", err, attackErr)
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
	ai.Think()

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
	ai.Think()

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
	ai.Think()

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
	ai.Think()

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
	ai.Think()

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
	ai.Think()

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
	ai.Think()

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
	ai.Think()

	if cast.castCalled {
		t.Fatal("Cast() called while still closing distance")
	}
	if move.followTarget != target || move.followRange != 400 {
		t.Fatalf("follow check = (%v, %d), want (%v, 400)", move.followTarget, move.followRange, target)
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
	ai.Think()

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
	ai.Think()

	if cast.castCalled {
		t.Fatal("Cast() called after the final cast gate rejected the attempt")
	}
	if owner.moveToPawnCalls != 1 || owner.moveToPawnTo != target {
		t.Fatalf("BroadcastMoveToPawn calls = (%d, %v), want (1, target)", owner.moveToPawnCalls, owner.moveToPawnTo)
	}
}

// TestAttackableThinkCastJoinsStopAndMoveToPawnBroadcastErrors is the
// regression test for the review finding that thinkCast's early
// `if pawnErr != nil { return pawnErr }` masked stopErr whenever both
// move.Stop's and BroadcastMoveToPawn's broadcasts failed in the same tick.
func TestAttackableThinkCastJoinsStopAndMoveToPawnBroadcastErrors(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	owner.known = map[int32]bool{target.ObjectID(): true}
	stopErr := errors.New("stop broadcast failed")
	pawnErr := errors.New("move-to-pawn broadcast failed")
	owner.moveToPawnErr = pawnErr
	move := &recordingMove{stopErr: stopErr}
	ref := skill.Ref{ID: 4, Level: 1}
	cast := &recordingCast{canAttempt: true, canCast: false, stopsMove: true}
	ai := NewAttackable(owner, move, &recordingAttack{})
	ai.SetCastController(cast)

	ai.Desires().AddOrUpdate(&Desire{Kind: IntentionCast, FinalTarget: target, Skill: ref, Weight: 10})
	err := ai.Think()

	if !errors.Is(err, stopErr) {
		t.Fatalf("Think() error = %v, want it to wrap stopErr (%v)", err, stopErr)
	}
	if !errors.Is(err, pawnErr) {
		t.Fatalf("Think() error = %v, want it to wrap pawnErr (%v)", err, pawnErr)
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
	ai.Think()

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
	ai.Think()

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
	ai.Think()

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

	ai.Think() // must not panic with no CastController wired.

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
	x, y, z         int
	headingTarget   attackable.Combatant
	moveToPawnCalls int
	moveToPawnTo    attackable.Combatant
	moveToPawnErr   error
}

func actor(id int32) *fakeActor {
	return &fakeActor{id: id, attackRange: 40, known: make(map[int32]bool), inTerritory: true}
}

func (a *fakeActor) ObjectID() int32  { return a.id }
func (a *fakeActor) SiegeGuard() bool { return a.siegeGuard }
func (a *fakeActor) AlikeDead() bool  { return a.alikeDead }
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
func (a *fakeActor) BroadcastMoveToPawn(target attackable.Combatant) error {
	a.moveToPawnCalls++
	a.moveToPawnTo = target
	return a.moveToPawnErr
}
func (a *fakeActor) ShouldIdleWander() bool { return a.idleWander }
func (a *fakeActor) ForceWalkStance()       { a.walkStanceCalls++ }
func (a *fakeActor) RealMoveSpeed() float64 { return a.moveSpeed }
func (a *fakeActor) MoveFromSpawnUsingRandomOffset(offset int) {
	a.wanderCalls++
	a.wanderOffset = offset
}

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
	stopErr       error
	home          location.Location
}

func (m *recordingMove) MaybeStartOffensiveFollow(target attackable.Combatant, attackRange int) (bool, error) {
	m.followCalls++
	m.followTarget = target
	m.followRange = attackRange
	return m.followStarted, nil
}

func (m *recordingMove) MoveHome(home location.Location) error {
	m.home = home
	return nil
}

func (m *recordingMove) Stop() error {
	m.stopCount++
	return m.stopErr
}

type recordingAttack struct {
	canAttack       bool
	canAttackTarget map[int32]bool
	attackingNow    bool
	bowCooling      bool
	target          attackable.Combatant
	doAttackCalls   int
	doAttackErr     error
}

func (a *recordingAttack) BowCoolingDown() bool { return a.bowCooling }
func (a *recordingAttack) AttackingNow() bool   { return a.attackingNow }
func (a *recordingAttack) CanAttack(target attackable.Combatant) bool {
	if a.canAttackTarget != nil {
		return a.canAttackTarget[target.ObjectID()]
	}
	return a.canAttack
}
func (a *recordingAttack) DoAttack(target attackable.Combatant) error {
	a.doAttackCalls++
	a.target = target
	return a.doAttackErr
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

	castCalled   bool
	castCalls    int
	castedTarget attackable.Combatant
	castedRef    skill.Ref
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

func (c *recordingCast) MeetsHPMPDisabled(target attackable.Combatant, ref skill.Ref) bool {
	return !c.hpMpFail
}

func (c *recordingCast) Cast(target attackable.Combatant, ref skill.Ref) {
	c.castCalled = true
	c.castCalls++
	c.castedTarget = target
	c.castedRef = ref
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

func TestAttackableThinkPrunesZeroWeightCastDesire(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	owner.known = map[int32]bool{target.ObjectID(): true}
	cast := &recordingCast{canAttempt: true, canCast: true}
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})
	ai.SetCastController(cast)
	ai.Desires().AddOrUpdate(&Desire{Kind: IntentionCast, FinalTarget: target, Skill: skill.Ref{ID: 4, Level: 1}, Weight: 0})

	ai.Think()

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

func TestAttackableThinkPrunesCastWhenHPMPDisabledFails(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	owner.known = map[int32]bool{target.ObjectID(): true}
	cast := &recordingCast{canAttempt: true, canCast: true, hpMpFail: true}
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})
	ai.SetCastController(cast)
	ai.Desires().AddOrUpdate(&Desire{Kind: IntentionCast, FinalTarget: target, Skill: skill.Ref{ID: 4, Level: 1}, Weight: 100})

	ai.Think()

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

func TestAttackableThinkPrunesAttackDesireBeyond1500(t *testing.T) {
	owner := actor(1)
	near := actor(2)
	far := actor(3)
	far.z = 1501
	owner.known = map[int32]bool{near.ObjectID(): true, far.ObjectID(): true}
	strike := &recordingAttack{canAttack: true}
	ai := NewAttackable(owner, &recordingMove{}, strike)
	addAttackHate(ai, far, 0, 100)
	addAttackHate(ai, near, 0, 50)

	ai.Think()

	if strike.target != near {
		t.Fatalf("attacked target = %v, want nearer attacker (far desire pruned)", strike.target)
	}
	if ai.Desires().Has(&Desire{Kind: IntentionAttack, FinalTarget: far}) {
		t.Fatal("far ATTACK desire still queued")
	}
}

func TestAttackableThinkKeepsAttackDesireAt1500(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	target.x = 1500
	owner.known = map[int32]bool{target.ObjectID(): true}
	strike := &recordingAttack{canAttack: true}
	ai := NewAttackable(owner, &recordingMove{}, strike)
	addAttackHate(ai, target, 0, 20)

	ai.Think()

	if strike.target != target {
		t.Fatalf("attacked target = %v, want target at exactly 1500", strike.target)
	}
}

func TestAttackableThinkKeepsFarAttackWhenOutOfControl(t *testing.T) {
	owner := actor(1)
	owner.denyAction = true
	far := actor(2)
	far.x = 2000
	owner.known = map[int32]bool{far.ObjectID(): true}
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{canAttack: true})
	addAttackHate(ai, far, 0, 20)

	ai.Think()

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
	ai.Think()
	if strike.target != target {
		t.Fatalf("first Think attacked = %v, want target", strike.target)
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

// TestAttackableAITickClearsStaleThreatOutOfTerritory ports NpcAI.java's
// out-of-territory fixed-rate task (NpcAI.java:298-339): a threat entry
// whose last damage is at least staleThreatAge old gets its hate stopped
// and its queued attack desire dropped once the owner has been out of
// territory for staleThreatSweepTicks.
func TestAttackableAITickClearsStaleThreatOutOfTerritory(t *testing.T) {
	owner := actor(1)
	owner.inTerritory = false
	target := actor(2)
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})
	addAttackHate(ai, target, 0, 20)

	future := time.Now().Add(91 * time.Second)
	ai.now = func() time.Time { return future }

	for i := 0; i < staleThreatSweepTicks; i++ {
		ai.Tick()
	}

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
	owner.inTerritory = false
	target := actor(2)
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})
	addAttackHate(ai, target, 0, 20)

	future := time.Now().Add(10 * time.Second)
	ai.now = func() time.Time { return future }

	for i := 0; i < staleThreatSweepTicks; i++ {
		ai.Tick()
	}

	if got := ai.Desires().Len(); got != 1 {
		t.Fatalf("desires len = %d, want 1 (fresh attack desire kept)", got)
	}
}

// TestAttackableAITickSkipsStaleSweepInTerritory confirms the sweep never
// runs while the owner is in its territory, matching NpcAI.java only
// starting the task on the isInMyTerritory() false transition and
// cancelling it back to territory.
func TestAttackableAITickSkipsStaleSweepInTerritory(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})
	addAttackHate(ai, target, 0, 20)

	future := time.Now().Add(91 * time.Second)
	ai.now = func() time.Time { return future }

	for i := 0; i < staleThreatSweepTicks; i++ {
		ai.Tick()
	}

	if got := ai.Desires().Len(); got != 1 {
		t.Fatalf("desires len = %d, want 1 (in-territory owner never runs the OOT sweep)", got)
	}
}

// TestAttackableAITickOutOfTerritorySweepRestartsOnReentry confirms
// returning to territory resets the sweep countdown, matching NpcAI.java
// cancelling the fixed-rate task on return and recreating it fresh (with
// its own initial delay) the next time the owner leaves territory.
func TestAttackableAITickOutOfTerritorySweepRestartsOnReentry(t *testing.T) {
	owner := actor(1)
	owner.inTerritory = false
	target := actor(2)
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})
	// A large hate value keeps the attack desire alive through the regular
	// per-3-tick decay (Attackable.Tick, attackHateDecay) across this
	// test's 19 total ticks, isolating the OOT-sweep-restart behavior under
	// test from that unrelated decay path.
	addAttackHate(ai, target, 0, 1000)

	future := time.Now().Add(91 * time.Second)
	ai.now = func() time.Time { return future }

	for i := 0; i < staleThreatSweepTicks-1; i++ {
		ai.Tick()
	}
	owner.inTerritory = true
	ai.Tick()
	owner.inTerritory = false

	for i := 0; i < staleThreatSweepTicks-1; i++ {
		ai.Tick()
	}

	if got := ai.Desires().Len(); got != 1 {
		t.Fatalf("desires len = %d, want 1 (sweep countdown restarted on territory reentry)", got)
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
	ai.Think()

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
	if got := ai.CurrentIntention(); got != IntentionWander {
		t.Fatalf("CurrentIntention() after reset = %v, want %v", got, IntentionWander)
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
	ai.Think()
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
	ai.Think()
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
	ai.Think()
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

	brain.SetWander()
	brain.AddMoveToDesire(home, 1_000_000)
	if err := brain.Think(); err != nil {
		t.Fatalf("Think() error: %v", err)
	}
	if got := brain.CurrentIntention(); got != IntentionMoveTo {
		t.Fatalf("CurrentIntention() after Think = %v, want %v", got, IntentionMoveTo)
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

func TestAttackableAIWanderReturnHome(t *testing.T) {
	owner := actor(1)
	owner.inTerritory = false
	owner.returnHome = true
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})

	ai.SetWander()
	ai.Think()

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

	ai.SetWander()
	ai.Think()

	if owner.returnHomeCalls != 0 {
		t.Fatalf("ReturnHome calls = %d, want 0 while moving", owner.returnHomeCalls)
	}
}

func TestAttackableAIWanderClearsWhenOutsideTerritoryAndNotReturning(t *testing.T) {
	owner := actor(1)
	owner.inTerritory = false
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})

	ai.SetWander()
	ai.Think()

	if got := ai.CurrentIntention(); got != IntentionIdle {
		t.Fatalf("CurrentIntention() = %v, want idle outside territory without return home", got)
	}
}

func TestAttackableAIWanderWalksFromSpawnOnFirstStep(t *testing.T) {
	owner := actor(1)
	owner.moveSpeed = 50
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})

	ai.SetWander()
	if err := ai.Think(); err != nil {
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

	if err := ai.Think(); err != nil {
		t.Fatalf("Think() error: %v", err)
	}

	if got := ai.CurrentIntention(); got != IntentionWander {
		t.Fatalf("CurrentIntention() = %v, want wander", got)
	}
	got, ok := ai.Desires().Peek()
	if !ok || got.Kind != IntentionWander || got.Timer != 5 || got.Weight != 5 {
		t.Fatalf("queued wander = (%v %+v), want timer 5 weight 5", ok, got)
	}
	if owner.wanderCalls != 1 || owner.wanderOffset != 120 {
		t.Fatalf("wander move = %d offset %d, want 1 call offset 120", owner.wanderCalls, owner.wanderOffset)
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

	ai.SetWander()
	if err := ai.Think(); err != nil {
		t.Fatalf("first Think() error: %v", err)
	}
	owner.wanderCalls = 0

	if err := ai.Think(); err != nil {
		t.Fatalf("arm-timer Think() error: %v", err)
	}
	if owner.wanderCalls != 0 {
		t.Fatalf("wander calls while timer arms = %d, want 0", owner.wanderCalls)
	}

	now = start.Add(4 * time.Second)
	if err := ai.Think(); err != nil {
		t.Fatalf("early Think() error: %v", err)
	}
	if owner.wanderCalls != 0 {
		t.Fatalf("wander calls before timer = %d, want 0", owner.wanderCalls)
	}

	now = start.Add(5 * time.Second)
	if err := ai.Think(); err != nil {
		t.Fatalf("due Think() error: %v", err)
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

	ai.SetWander()
	if err := ai.Think(); err != nil {
		t.Fatalf("first Think() error: %v", err)
	}
	owner.wanderCalls = 0
	if err := ai.Think(); err != nil {
		t.Fatalf("arm-timer Think() error: %v", err)
	}

	now = start.Add(5 * time.Second)
	if err := ai.Think(); err != nil {
		t.Fatalf("due Think() error: %v", err)
	}
	if owner.wanderCalls != 0 {
		t.Fatalf("wander calls with rate 0 = %d, want 0", owner.wanderCalls)
	}

	now = start.Add(9 * time.Second)
	if err := ai.Think(); err != nil {
		t.Fatalf("before second timer Think() error: %v", err)
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

	ai.SetWander()
	if err := ai.Think(); err != nil {
		t.Fatalf("wander Think() error: %v", err)
	}

	addAttackHate(ai, target, 0, 10)
	if err := ai.Think(); err != nil {
		t.Fatalf("attack Think() error: %v", err)
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

// TestSummonThinkLogsBothStopAndAttackBroadcastErrors is the regression
// test for the review finding that thinkAttackLocked's masking pattern
// (mirroring Attackable.thinkAttack) dropped attackErr from the log
// whenever both move.Stop's and attack.DoAttack's broadcasts failed in the
// same tick.
func TestSummonThinkLogsBothStopAndAttackBroadcastErrors(t *testing.T) {
	owner := actor(100)
	target := actor(200)
	stopErr := errors.New("stop broadcast failed")
	attackErr := errors.New("attack broadcast failed")
	move := &summonMove{}
	move.stopErr = stopErr
	strike := &recordingAttack{canAttack: true, doAttackErr: attackErr}
	brain := NewSummon(owner, move, strike)
	var buf bytes.Buffer
	brain.SetLogger(zerolog.New(&buf))

	if !brain.TryToAttack(target) {
		t.Fatal("TryToAttack() = false, want accepted attack")
	}

	logged := buf.String()
	if !strings.Contains(logged, stopErr.Error()) {
		t.Fatalf("logged error = %q, want it to contain stopErr (%v)", logged, stopErr)
	}
	if !strings.Contains(logged, attackErr.Error()) {
		t.Fatalf("logged error = %q, want it to contain attackErr (%v)", logged, attackErr)
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

	if !brain.TryToCast(target, ref) {
		t.Fatal("TryToCast() = false, want accepted cast")
	}
	if cast.castedTarget != target || cast.castedRef != ref {
		t.Fatalf("cast = (%v,%v), want (%v,%v)", cast.castedTarget, cast.castedRef, target, ref)
	}
	if got := brain.CurrentIntention(); got != IntentionIdle {
		t.Fatalf("CurrentIntention() = %v, want idle once the cast is dispatched", got)
	}
}

func TestSummonAITryToCastApproachesBeforeCasting(t *testing.T) {
	owner := actor(100)
	target := actor(200)
	move := &summonMove{recordingMove: recordingMove{followStarted: true}}
	cast := &recordingCast{canAttempt: true, canCast: true, castRange: 400}
	brain := NewSummon(owner, move, &recordingAttack{})
	brain.SetCastController(cast)

	brain.TryToCast(target, skill.Ref{ID: 4139, Level: 8})
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

	brain.TryToCast(target, skill.Ref{ID: 4139, Level: 8})
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

	if !brain.TryToCast(target, skill.Ref{ID: 4139, Level: 8}) {
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

	if !brain.TryToCast(target, ref) {
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

	if brain.TryToCast(actor(200), skill.Ref{ID: 4139, Level: 8}) {
		t.Fatal("TryToCast() = true with no CastController attached, want false")
	}
}

func TestSummonAITryToCastRejectsWhenCanAttemptFails(t *testing.T) {
	target := actor(200)
	cast := &recordingCast{canAttempt: false, canCast: true}
	brain := NewSummon(actor(100), &summonMove{}, &recordingAttack{})
	brain.SetCastController(cast)

	if brain.TryToCast(target, skill.Ref{ID: 4139, Level: 8}) {
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

	if !brain.TryToCast(target, ref) {
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

	brain.Think()
	if got := brain.CurrentIntention(); got != IntentionFollow {
		t.Fatalf("CurrentIntention() = %v, want %v", got, IntentionFollow)
	}
	if owner.thinkCalls != 0 {
		t.Fatalf("ThinkFollow calls on first pulse = %d, want 0 (even pulse skips)", owner.thinkCalls)
	}

	brain.Think()
	if owner.thinkCalls != 1 {
		t.Fatalf("ThinkFollow calls on second pulse = %d, want 1", owner.thinkCalls)
	}
}

func TestAttackableAttackDesireReplacesFollow(t *testing.T) {
	owner := &followStub{fakeActor: actor(1), idleTarget: actor(9)}
	target := actor(2)
	owner.known[target.ObjectID()] = true
	strike := &recordingAttack{canAttack: true}
	brain := NewAttackable(owner, &recordingMove{}, strike)

	brain.Think()
	if got := brain.CurrentIntention(); got != IntentionFollow {
		t.Fatalf("CurrentIntention() after idle = %v, want %v", got, IntentionFollow)
	}

	brain.AddDamageHate(target, 0, 200)
	brain.AddAttackDesire(target, 200)
	brain.Think()
	if got := brain.CurrentIntention(); got != IntentionAttack {
		t.Fatalf("CurrentIntention() after attack desire = %v, want %v", got, IntentionAttack)
	}
	if strike.target != target {
		t.Fatalf("attack target = %v, want the queued attacker", strike.target)
	}
}
