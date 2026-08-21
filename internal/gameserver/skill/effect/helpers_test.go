package effect

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/worldobject"
)

// namedActor is a minimal participant with a fixed String() so tests that
// only need a filler Effector/Effected (not a specific capability) get a
// stable %v representation instead of a struct address.
type namedActor string

func (namedActor) ObjectID() int32  { return 0 }
func (namedActor) Dead() bool       { return false }
func (n namedActor) String() string { return string(n) }

type liveEffectTarget struct {
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
	target            worldobject.Object
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
	mpBroadcasts      int
}

func (t *liveEffectTarget) BroadcastMPStatus() { t.mpBroadcasts++ }

func (t *liveEffectTarget) EffectList() *List { return t.list }

func (t *liveEffectTarget) CancelVulnerability(classification string) float64 { return t.vuln }

func (t *liveEffectTarget) Dead() bool { return t.dead }

func (t *liveEffectTarget) HP() float64 { return t.hp }

func (t *liveEffectTarget) MPValue() float64 { return t.mp }

func (t *liveEffectTarget) ReduceHPByDOT(damage float64, effector Participant, isDOT bool) {
	t.hp -= damage
	t.events = append(t.events, fmt.Sprintf("dot:%g:%v", damage, effector))
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

func (t *liveEffectTarget) Afraid() bool { return t.afraid }

func (t *liveEffectTarget) FearImmune() bool { return t.fearImmune }

func (t *liveEffectTarget) Playable() bool { return t.playable }

func (t *liveEffectTarget) FleeFrom(effector Participant, distance int) {
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

func (t *liveEffectTarget) SetInvul(v bool) {
	t.events = append(t.events, fmt.Sprintf("invul:%v", v))
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

func (t *liveEffectTarget) CurrentTarget() worldobject.Object { return t.target }

func (t *liveEffectTarget) SetTarget(target worldobject.Object) {
	t.target = target
	t.events = append(t.events, fmt.Sprintf("set-target:%v", target))
}

func (t *liveEffectTarget) TryToAttack(target worldobject.Object) {
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

// noBonusHealTarget implements only the minimum heal capability, to
// exercise the healStart/manaHealStart fallback defaults when the optional

func hasEffectInList(list *List, e *Effect) bool {
	for _, cur := range list.All() {
		if cur == e {
			return true
		}
	}
	return false
}
