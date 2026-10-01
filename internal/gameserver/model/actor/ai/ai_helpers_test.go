package ai

import (
	"testing"
	"time"

	modelactor "github.com/fatal10110/acis_golang/internal/gameserver/model/actor"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable/attackabletest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
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

func thinkWanderOnce(ai *Attackable) error {
	ai.Desires().AddOrUpdate(&Desire{Kind: IntentionWander, Timer: 5, Weight: 5})
	return ai.RunAI()
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
	confused        bool
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
	timers          []*fakeTimer
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
func (a *fakeActor) OutOfControl() bool { return a.denyAction || a.confused }

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

// fakeTimer is a task armed by fakeActor.After; tests run it by hand.
type fakeTimer struct {
	delay   time.Duration
	fn      func()
	stopped bool
}

func (t *fakeTimer) Stop() bool {
	was := t.stopped
	t.stopped = true
	return !was
}

func (a *fakeActor) After(d time.Duration, fn func()) Timer {
	t := &fakeTimer{delay: d, fn: fn}
	a.timers = append(a.timers, t)
	return t
}

// pendingTimers returns the armed timers not yet stopped or run.
func (a *fakeActor) pendingTimers() []*fakeTimer {
	var pending []*fakeTimer
	for _, t := range a.timers {
		if !t.stopped {
			pending = append(pending, t)
		}
	}
	return pending
}

// fireTimer runs the sole pending timer, failing unless exactly one is
// armed with delay want.
func (a *fakeActor) fireTimer(t *testing.T, want time.Duration) {
	t.Helper()
	pending := a.pendingTimers()
	if len(pending) != 1 {
		t.Fatalf("pending timers = %d, want 1", len(pending))
	}
	if pending[0].delay != want {
		t.Fatalf("timer delay = %v, want %v", pending[0].delay, want)
	}
	pending[0].stopped = true
	pending[0].fn()
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

func tickOOTSweepDue(ai *Attackable, at time.Time) {
	ai.now = func() time.Time { return at }
	ai.Tick()
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

func (recordingMove) MoveToLocation(location.Location) (bool, error) { return false, nil }

func (*fakeActor) IdleFollowTarget() attackable.Combatant { return nil }

func (*fakeActor) ThinkFollow(attackable.Combatant, bool) bool { return false }

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
