package ai

import (
	"math"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

func TestAttackableAIChoosesMostHatedTargetToAttack(t *testing.T) {
	owner := actor(1)
	low := actor(2)
	high := actor(3)
	owner.known = map[int32]bool{low.ObjectID(): true, high.ObjectID(): true}
	owner.attackRange = 40
	move := &recordingMove{}
	strike := &recordingAttack{canAttack: true}
	ai := NewAttackable(owner, move, strike)

	ai.AddDamageHate(low, 0, 10)
	ai.AddDamageHate(high, 0, 25)
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

func TestAttackableAIStartsOffensiveFollowBeforeAttack(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	owner.known = map[int32]bool{target.ObjectID(): true}
	owner.attackRange = 80
	move := &recordingMove{followStarted: true}
	strike := &recordingAttack{canAttack: true}
	ai := NewAttackable(owner, move, strike)

	ai.AddDamageHate(target, 0, 100)
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

	ai.AddDamageHate(target, 0, 100)
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

	ai.AddDamageHate(target, 0, 100)
	ai.Think()

	if move.followTarget != nil {
		t.Fatalf("follow target = %v, want none for lost target", move.followTarget)
	}
	if strike.target != nil {
		t.Fatalf("attacked target = %v, want none for lost target", strike.target)
	}
}

func TestAttackableAIReselectsWhenTopAttackTargetCannotBeUsed(t *testing.T) {
	owner := actor(1)
	blocked := actor(2)
	reachable := actor(3)
	owner.known = map[int32]bool{blocked.ObjectID(): true, reachable.ObjectID(): true}
	move := &recordingMove{}
	strike := &recordingAttack{
		canAttackTarget: map[int32]bool{
			blocked.ObjectID():   false,
			reachable.ObjectID(): true,
		},
	}
	ai := NewAttackable(owner, move, strike)

	ai.AddDamageHate(reachable, 0, 25)
	ai.AddDamageHate(blocked, 0, 100)
	ai.Think()

	if strike.target != reachable {
		t.Fatalf("attacked target = %v, want reachable fallback target", strike.target)
	}
	if got := ai.Threats().Hate(blocked); got != 0 {
		t.Fatalf("blocked target hate = %v, want stopped", got)
	}
	if got := ai.Threats().Hate(reachable); got <= 25 {
		t.Fatalf("reachable target hate = %v, want reselection hate transferred above original 25", got)
	}
}

func TestAttackableAITickDecaysThreatEveryThirdTick(t *testing.T) {
	owner := actor(1)
	target := actor(2)
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})
	ai.AddDamageHate(target, 0, 20)

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

func TestAttackableAITickRefreshesStaleThreatAndHate(t *testing.T) {
	owner := actor(1)
	lost := actor(2)
	dead := actor(3)
	dead.alikeDead = true
	kept := actor(4)
	owner.known = map[int32]bool{lost.ObjectID(): false, dead.ObjectID(): true, kept.ObjectID(): true}
	ai := NewAttackable(owner, &recordingMove{}, &recordingAttack{})
	ai.AddDamageHate(lost, 7, 70)
	ai.AddDamageHate(dead, 8, 80)
	ai.AddDamageHate(kept, 9, 90)
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
	if got := ai.Hates().Hate(kept); got != -65100 {
		t.Fatalf("kept hate entry = %v, want decayed", got)
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

	ai.AddDamageHate(target, 5, 20)
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
	headingTarget   attackable.Combatant
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
func (a *fakeActor) InTerritory() bool { return a.inTerritory }
func (a *fakeActor) SetHeadingTo(target attackable.Combatant) {
	a.headingTarget = target
}

type recordingMove struct {
	followStarted bool
	followTarget  attackable.Combatant
	followRange   int
	stopCount     int
}

func (m *recordingMove) MaybeStartOffensiveFollow(target attackable.Combatant, attackRange int) bool {
	m.followTarget = target
	m.followRange = attackRange
	return m.followStarted
}

func (m *recordingMove) MoveHome(location.Location) {}

func (m *recordingMove) Stop() {
	m.stopCount++
}

type recordingAttack struct {
	canAttack       bool
	canAttackTarget map[int32]bool
	attackingNow    bool
	bowCooling      bool
	target          attackable.Combatant
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
	a.target = target
}

type recordingCast struct {
	disabled   bool
	canAttempt bool
	canCast    bool
	stopsMove  bool
	castRange  int

	castCalled   bool
	castedTarget attackable.Combatant
	castedRef    skill.Ref
}

func (c *recordingCast) Disabled() bool               { return c.disabled }
func (c *recordingCast) Range(ref skill.Ref) int      { return c.castRange }
func (c *recordingCast) StopsMovement(skill.Ref) bool { return c.stopsMove }

func (c *recordingCast) CanAttempt(target attackable.Combatant, ref skill.Ref) bool {
	return c.canAttempt
}

func (c *recordingCast) CanCast(target attackable.Combatant, ref skill.Ref) bool {
	return c.canCast
}

func (c *recordingCast) Cast(target attackable.Combatant, ref skill.Ref) {
	c.castCalled = true
	c.castedTarget = target
	c.castedRef = ref
}
