package ai

import (
	"math"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
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
	return a.known[target.ObjectID()]
}
func (a *fakeActor) PhysicalAttackRange() int { return a.attackRange }
func (a *fakeActor) ReturnHome() bool {
	a.returnHomeCalls++
	return a.returnHome
}
func (a *fakeActor) InTerritory() bool { return a.inTerritory }

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
	canAttack    bool
	attackingNow bool
	bowCooling   bool
	target       attackable.Combatant
}

func (a *recordingAttack) BowCoolingDown() bool { return a.bowCooling }
func (a *recordingAttack) AttackingNow() bool   { return a.attackingNow }
func (a *recordingAttack) CanAttack(target attackable.Combatant) bool {
	return a.canAttack
}
func (a *recordingAttack) DoAttack(target attackable.Combatant) {
	a.target = target
}
