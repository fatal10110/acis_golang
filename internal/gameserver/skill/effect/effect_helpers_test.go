package effect

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

type liveEffectTarget struct {
	neutralActor
	world.Presence
	events            []string
	hp                float64
	mp                float64
	dead              bool
	afraid            bool
	fearImmune        bool
	playable          bool
	raidRelated       bool
	castingNow        bool
	castMagic         bool
	canBeHealed       bool
	healProficiency   float64
	healEffectiveness float64
	rechargeRate      func(float64) float64
	target            world.Tracked
	heading           int
	bluffExempt       bool
	isPlayer          bool
	list              *List
	vuln              float64
	standing          bool
	hpFull            bool
	relaxNotice       int
	recentFakeDeath   bool
	objectID          int32
	ownerID           int32
	x, y, z           int
	validLocationFn   func(ox, oy, oz, tx, ty, tz int) location.Location
	flightDest        location.Location
	flightType        modelskill.Flight
}

func (t *liveEffectTarget) EffectList() *List { return t.list }

func (t *liveEffectTarget) CancelVulnerability(classification string) float64 { return t.vuln }

func (t *liveEffectTarget) Dead() bool { return t.dead }

func (t *liveEffectTarget) HP() float64 { return t.hp }

func (t *liveEffectTarget) MPValue() float64 { return t.mp }

func (t *liveEffectTarget) ReduceHPByDOT(damage float64, effector Actor, isDOT bool) {
	t.hp -= damage
	t.events = append(t.events, fmt.Sprintf("dot:%g:%v", damage, effector))
}

func (t *liveEffectTarget) ReduceHPByToggleUpkeep(damage float64, effector Actor) {
	t.hp -= damage
	t.events = append(t.events, fmt.Sprintf("upkeep:%g:%v", damage, effector))
}

// ReduceMP mirrors the production actors' clamp-at-zero semantics (see
// Character.ReduceMP/Hostile.ReduceMP): a target already at 0 MP applies
// and returns 0 rather than going negative, so tests can exercise the
// "nothing to apply, don't broadcast" guard alongside the real reducers.
func (t *liveEffectTarget) ReduceMP(damage float64) float64 {
	if damage <= 0 || t.mp <= 0 {
		return 0
	}
	if damage > t.mp {
		damage = t.mp
	}
	t.mp -= damage
	t.events = append(t.events, fmt.Sprintf("mpdot:%g", damage))
	return damage
}

func (t *liveEffectTarget) NotifyEffectRemovedDueLackHP(*Effect) {
	t.events = append(t.events, "lack-hp")
}

func (t *liveEffectTarget) NotifyEffectRemovedDueLackMP(*Effect) {
	t.events = append(t.events, "lack-mp")
}

func (t *liveEffectTarget) AbortAll(force bool) {
	t.events = append(t.events, fmt.Sprintf("abort:%v", force))
}

func (t *liveEffectTarget) TryToIdle() {
	t.events = append(t.events, "idle")
}

func (t *liveEffectTarget) StopMove() {
	t.events = append(t.events, "stop-move")
}

func (t *liveEffectTarget) UpdateAbnormalEffect() {
	t.events = append(t.events, "abnormal")
}

func (t *liveEffectTarget) Think() error {
	t.events = append(t.events, "think")
	return nil
}

func (t *liveEffectTarget) WakeAI() { t.events = append(t.events, "wake-player-ai") }

func (t *liveEffectTarget) Afraid() bool { return t.afraid }

func (t *liveEffectTarget) FearImmune() bool { return t.fearImmune }

func (t *liveEffectTarget) Playable() bool { return t.playable }

func (t *liveEffectTarget) FleeFrom(effector Actor, distance int) {
	t.events = append(t.events, fmt.Sprintf("flee:%v:%d", effector, distance))
}

func (t *liveEffectTarget) StopEffects(typ Type) {
	t.events = append(t.events, "stop-effects:"+string(typ))
}

func (t *liveEffectTarget) RaidRelated() bool { return t.raidRelated }

func (t *liveEffectTarget) CastingNow() bool { return t.castingNow }

func (t *liveEffectTarget) CurrentSkillIsMagic() bool { return t.castMagic }

func (t *liveEffectTarget) InterruptCast() {
	t.events = append(t.events, "interrupt-cast")
}

func (t *liveEffectTarget) StopCast() {
	t.events = append(t.events, "stop-cast")
}

func (t *liveEffectTarget) ClearTarget() {
	t.events = append(t.events, "clear-target")
}

func (t *liveEffectTarget) StopAttack() {
	t.events = append(t.events, "stop-attack")
}

func (t *liveEffectTarget) SetInvul(v bool) bool {
	t.events = append(t.events, fmt.Sprintf("invul:%v", v))
	return true
}

func (t *liveEffectTarget) SetImmobilized(v bool) bool {
	t.events = append(t.events, fmt.Sprintf("immobilized:%v", v))
	return true
}

func (t *liveEffectTarget) CanBeHealed() bool { return t.canBeHealed }

func (t *liveEffectTarget) AddMP(amount float64) float64 {
	t.mp += amount
	t.events = append(t.events, fmt.Sprintf("add-mp:%g", amount))
	return amount
}

func (t *liveEffectTarget) AddHP(amount float64) float64 {
	t.hp += amount
	t.events = append(t.events, fmt.Sprintf("add-hp:%g", amount))
	return amount
}

func (t *liveEffectTarget) HealProficiency() float64 { return t.healProficiency }

func (t *liveEffectTarget) HealEffectiveness() float64 { return t.healEffectiveness }

func (t *liveEffectTarget) RechargeMP(base float64) float64 {
	if t.rechargeRate == nil {
		return base
	}
	return t.rechargeRate(base)
}

func (t *liveEffectTarget) CurrentTarget() world.Tracked { return t.target }

func (t *liveEffectTarget) SetTarget(target world.Tracked) {
	t.target = target
	t.events = append(t.events, fmt.Sprintf("set-target:%v", target))
}

func (t *liveEffectTarget) TryToAttack(target world.Tracked) {
	t.events = append(t.events, fmt.Sprintf("try-attack:%v", target))
}

func (t *liveEffectTarget) Heading() int { return t.heading }

func (t *liveEffectTarget) SetHeading(h int) {
	t.heading = h
	t.events = append(t.events, fmt.Sprintf("heading:%d", h))
}

func (t *liveEffectTarget) BluffExempt() bool { return t.bluffExempt }

func (t *liveEffectTarget) IsPlayer() bool { return t.isPlayer }

func (t *liveEffectTarget) StopCharmOfLuck(*Effect) {
	t.events = append(t.events, "stop-charm-of-luck")
}

func (t *liveEffectTarget) StopPhoenixBlessing(*Effect) {
	t.events = append(t.events, "stop-phoenix-bless")
}

func (t *liveEffectTarget) StopProtectionBlessing(*Effect) {
	t.events = append(t.events, "stop-protection-bless")
}

func (t *liveEffectTarget) BroadcastEtcStatus() {
	t.events = append(t.events, "etc-status")
}

func (t *liveEffectTarget) StopSkillEffectsByID(id modelskill.ID) {
	t.events = append(t.events, fmt.Sprintf("stop-skill:%d", id))
}

func (t *liveEffectTarget) Standing() bool { return t.standing }

func (t *liveEffectTarget) SetStanding(v bool) bool {
	changed := t.standing != v
	t.standing = v
	t.events = append(t.events, fmt.Sprintf("standing:%v", v))
	return changed
}

func (t *liveEffectTarget) HPFull() bool { return t.hpFull }

func (t *liveEffectTarget) NotifyRelaxDeactivatedHPFull(*Effect) {
	t.relaxNotice++
}

func (t *liveEffectTarget) MarkRecentFakeDeath() {
	t.recentFakeDeath = true
	t.events = append(t.events, "recent-fake-death")
}

func (t *liveEffectTarget) ObjectID() int32 { return t.objectID }

func (t *liveEffectTarget) OwnerID() int32 { return t.ownerID }

func (t *liveEffectTarget) X() int { return t.x }
func (t *liveEffectTarget) Y() int { return t.y }
func (t *liveEffectTarget) Z() int { return t.z }

func (t *liveEffectTarget) ValidLocation(ox, oy, oz, tx, ty, tz int) location.Location {
	if t.validLocationFn != nil {
		return t.validLocationFn(ox, oy, oz, tx, ty, tz)
	}
	return location.Location{X: tx, Y: ty, Z: tz}
}

func (t *liveEffectTarget) FlyTo(dest location.Location, flight modelskill.Flight) {
	t.flightDest = dest
	t.flightType = flight
	t.events = append(t.events, "fly")
}

func (t *liveEffectTarget) SetXYZ(x, y, z int) {
	t.x, t.y, t.z = x, y, z
}

func (t *liveEffectTarget) BroadcastPosition() {
	t.events = append(t.events, "broadcast")
}

// ---- from list_test.go ----
type eventOwner struct {
	events  *[]string
	maxBuff int
}

func (o eventOwner) AttachStatFuncs([]Mod) {
	*o.events = append(*o.events, "owner:add")
}

func (o eventOwner) StatFuncsAttached([]Mod) {}

func (o eventOwner) RemoveStatsByOwner(owner ModOwner) {
	e := owner.effect
	*o.events = append(*o.events, "owner:remove:"+e.Template.Name)
}

func (o eventOwner) MaxBuffCount() int {
	if o.maxBuff == 0 {
		return 20
	}
	return o.maxBuff
}

func (o eventOwner) NotifyEffectWornOff(skillID modelskill.ID, level int) {
	*o.events = append(*o.events, fmt.Sprintf("worn-off:%d:%d", skillID, level))
}

func (o eventOwner) NotifyEffectDisappeared(skillID modelskill.ID, level int) {
	*o.events = append(*o.events, fmt.Sprintf("disappeared:%d:%d", skillID, level))
}

func (o eventOwner) NotifyEffectAborted(skillID modelskill.ID, level int) {
	*o.events = append(*o.events, fmt.Sprintf("aborted:%d:%d", skillID, level))
}

func (o eventOwner) NotifyEffectFelt(skillID modelskill.ID, level int) {
	*o.events = append(*o.events, fmt.Sprintf("felt:%d:%d", skillID, level))
}

func newEffect(name string, id modelskill.ID, stackType string, stackOrder float64, debuff bool) *Effect {
	e := &Effect{
		Skill: Skill{
			ID:        id,
			StackType: stackType,
			Debuff:    debuff,
		},
		Template: modelskill.EffectTemplate{
			Name:       name,
			StackType:  stackType,
			StackOrder: stackOrder,
		},
		Type: TypeBuff,
	}
	e.OnStart = func(*Effect) bool {
		e.Template.Value++
		return true
	}
	return e
}

func namedEffect(name string, id modelskill.ID, stackType string, stackOrder float64, debuff bool, events *[]string) *Effect {
	e := newEffect(name, id, stackType, stackOrder, debuff)
	e.OnStart = func(*Effect) bool {
		*events = append(*events, name+":start")
		return true
	}
	e.OnExit = func(*Effect) {
		*events = append(*events, name+":exit")
	}
	e.OnStopTask = func(*Effect) {
		*events = append(*events, name+":stop")
	}
	return e
}

func effectNames(effects []*Effect) []string {
	names := make([]string, len(effects))
	for i, e := range effects {
		names[i] = e.Template.Name
	}
	return names
}

func requireEvents(t *testing.T, got []string, want []string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("events = %#v, want %#v", got, want)
	}
}

func requireNames(t *testing.T, got []*Effect, want []string) {
	t.Helper()
	if names := effectNames(got); !reflect.DeepEqual(names, want) {
		t.Fatalf("effects = %#v, want %#v", names, want)
	}
}

// buffSlotEffect returns a named, non-stacking buff-slot-family effect
// (the family of skill types that occupy an owner's limited buff slots and
// are shown as an icon).
func buffSlotEffect(name string, id modelskill.ID, events *[]string) *Effect {
	e := namedEffect(name, id, "none", 0, false, events)
	e.Skill.SkillType = "BUFF"
	e.Template.Icon = true
	return e
}

func (eventOwner) UpdateEffectIcons() {}

var (
	_ PlayerActor = (*liveEffectTarget)(nil)
	_ NPCActor    = (*liveEffectTarget)(nil)
	_ SummonActor = (*liveEffectTarget)(nil)
)

func (t *liveEffectTarget) IncreaseCharges(int, int) bool                        { return false }
func (t *liveEffectTarget) WeaponGradePenalty() bool                             { return false }
func (t *liveEffectTarget) ReduceDeathPenaltyLevel() int                         { return 0 }
func (t *liveEffectTarget) Sit() bool                                            { return false }
func (t *liveEffectTarget) StartFakeDeath() bool                                 { return false }
func (t *liveEffectTarget) StopFakeDeath() bool                                  { return false }
func (t *liveEffectTarget) BroadcastStatus()                                     {}
func (t *liveEffectTarget) SendRegenMax(int32, int32, float64)                   {}
func (t *liveEffectTarget) NotifyHPRestored(string, int, bool)                   {}
func (t *liveEffectTarget) NotifyMPRestored(string, int, bool)                   {}
func (t *liveEffectTarget) NotifySpoilAlready()                                  {}
func (t *liveEffectTarget) NotifySpoilSuccess()                                  {}
func (t *liveEffectTarget) AddDamageHate(attackable.Combatant, float64, float64) {}
func (t *liveEffectTarget) AddAttackDesire(attackable.Combatant, float64)        {}
func (t *liveEffectTarget) MonsterKind() bool                                    { return false }
func (t *liveEffectTarget) RandomNearbyMonster(int) (attackable.Combatant, bool) { return nil, false }

func (t *liveEffectTarget) RandomNearbyCombatant(int) (attackable.Combatant, bool) { return nil, false }
func (t *liveEffectTarget) RandomizeHate() bool                                    { return false }
func (t *liveEffectTarget) StopMostHatedTarget()                                   {}
func (t *liveEffectTarget) SpoilPool() *item.SpoilPool                             { return nil }
func (t *liveEffectTarget) CollisionRadius() float64                               { return 0 }
func (t *liveEffectTarget) SetCollisionRadius(float64)                             {}
func (t *liveEffectTarget) ResetCollisionRadius()                                  {}
func (t *liveEffectTarget) OwnerObject() (world.Tracked, bool)                     { return nil, false }
func (t *liveEffectTarget) TryToFollow(world.Tracked)                              {}
func (t *liveEffectTarget) RandomConfusionTarget(int) (world.Tracked, bool)        { return nil, false }

func (t *liveEffectTarget) Kind() actor.Kind {
	switch {
	case t.isPlayer:
		return actor.KindPlayer
	case t.playable:
		return actor.KindSummon
	}
	return actor.KindNPC
}
