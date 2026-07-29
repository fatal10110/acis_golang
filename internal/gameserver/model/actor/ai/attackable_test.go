package ai

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

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
	moveToPawnCalls int
	moveToPawnTo    attackable.Combatant
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
func (a *fakeActor) BroadcastMoveToPawn(target attackable.Combatant) {
	a.moveToPawnCalls++
	a.moveToPawnTo = target
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
	skillType  string

	castCalled   bool
	castedTarget attackable.Combatant
	castedRef    skill.Ref
}

func (c *recordingCast) Disabled() bool               { return c.disabled }
func (c *recordingCast) Range(ref skill.Ref) int      { return c.castRange }
func (c *recordingCast) StopsMovement(skill.Ref) bool { return c.stopsMove }
func (c *recordingCast) SkillType(skill.Ref) string   { return c.skillType }

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
