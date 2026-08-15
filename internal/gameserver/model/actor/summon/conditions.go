package summon

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/conditions"
)

// summonStatActor implements conditions.Actor, backed by *Actor's real
// position/heading/active-effect state, mirroring characterStatActor
// (model/actor/player/character_conditions.go). Summons are never
// conditions.PlayerActor, matching the Java reference
// (ConditionUsingItemType/player-state conditions require
// effector instanceof Player): a using/player/resting-tagged condition on a
// summon owner correctly fails closed via conditionGate's type assertion,
// not error. See #1509.
var _ conditions.Actor = summonStatActor{}

// HPRatio satisfies conditions.Actor.
func (s summonStatActor) HPRatio() float64 {
	max := s.a.MaxHPValue()
	if max <= 0 {
		return 0
	}
	return s.a.HP() / max
}

// MPRatio satisfies conditions.Actor.
func (s summonStatActor) MPRatio() float64 {
	max := s.a.MaxMPValue()
	if max <= 0 {
		return 0
	}
	return s.a.MPValue() / max
}

// X satisfies conditions.Actor.
func (s summonStatActor) X() int { return s.a.X() }

// Y satisfies conditions.Actor.
func (s summonStatActor) Y() int { return s.a.Y() }

// Z satisfies conditions.Actor.
func (s summonStatActor) Z() int { return s.a.Z() }

// IsMoving satisfies conditions.Actor. Always false: unlike *player.Character
// and *npc.Hostile, *Actor carries no move controller or in-motion state at
// all yet (tracked in #1510), so there is nothing to report.
func (s summonStatActor) IsMoving() bool { return false }

// IsRunning satisfies conditions.Actor. Always true, matching
// hostileStatActor: Java's Creature walk/run toggle defaults to run stance
// and nothing puts a non-player actor back into walk stance.
func (s summonStatActor) IsRunning() bool { return true }

// IsRiding satisfies conditions.Actor. Always false: summons are never
// mounted, matching Creature.isRiding's un-overridden default.
func (s summonStatActor) IsRiding() bool { return false }

// IsFlying satisfies conditions.Actor. Always false: no shipped summon
// overrides Creature.isFlying's default.
func (s summonStatActor) IsFlying() bool { return false }

// headingActor is the extra capability IsBehind/IsInFrontOf need beyond
// conditions.Actor's plain X/Y/Z: the reference position's own heading.
// conditionGate always tests an owner against itself
// (conditionGate.Test(actor, actor, nil)), so other is always this same
// summonStatActor, which implements it via CurrentHeading below.
type headingActor interface{ CurrentHeading() int }

// CurrentHeading lets a summonStatActor serve as the "other" side of its
// own IsBehind/IsInFrontOf check.
func (s summonStatActor) CurrentHeading() int { return s.a.Heading() }

// IsBehind satisfies conditions.Actor: reports whether s is positioned
// behind other, using other's own facing (matching
// creature.ResolveBlowInput's identical behind/front check).
func (s summonStatActor) IsBehind(other conditions.Actor) bool {
	h, ok := other.(headingActor)
	if !ok {
		return false
	}
	facing := location.OrientedLocation{
		Location: location.Location{X: other.X(), Y: other.Y(), Z: other.Z()},
		Heading:  h.CurrentHeading(),
	}
	return facing.IsBehind(location.Location{X: s.X(), Y: s.Y(), Z: s.Z()})
}

// IsInFrontOf satisfies conditions.Actor: reports whether s is positioned in
// front of other, using other's own facing.
func (s summonStatActor) IsInFrontOf(other conditions.Actor) bool {
	h, ok := other.(headingActor)
	if !ok {
		return false
	}
	facing := location.OrientedLocation{
		Location: location.Location{X: other.X(), Y: other.Y(), Z: other.Z()},
		Heading:  h.CurrentHeading(),
	}
	return facing.IsInFrontOf(location.Location{X: s.X(), Y: s.Y(), Z: s.Z()})
}

// ActiveSkillLevel satisfies conditions.Actor.
func (s summonStatActor) ActiveSkillLevel(id int) (int, bool) {
	return s.a.EffectList().ActiveBySkillID(id)
}

// ActiveEffectLevel satisfies conditions.Actor.
func (s summonStatActor) ActiveEffectLevel(effectID int) (int, bool) {
	return s.a.EffectList().ActiveBySkillID(effectID)
}
