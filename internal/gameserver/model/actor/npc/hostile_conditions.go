package npc

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/conditions"
)

// hostileStatActor implements conditions.Actor, backed by *Hostile's real
// position/movement/heading/active-effect state, mirroring
// characterStatActor (model/actor/player/character_conditions.go). NPCs are
// never conditions.PlayerActor, matching the Java reference
// (ConditionUsingItemType/player-state conditions require
// effector instanceof Player): a using/player/resting-tagged condition on
// an NPC owner correctly fails closed via conditionGate's type assertion,
// not error. See #1509.
var _ conditions.Actor = hostileStatActor{}

// HPRatio satisfies conditions.Actor.
func (a hostileStatActor) HPRatio() float64 {
	max := a.h.MaxHPValue()
	if max <= 0 {
		return 0
	}
	return a.h.HP() / max
}

// MPRatio satisfies conditions.Actor.
func (a hostileStatActor) MPRatio() float64 {
	max := a.h.MaxMPValue()
	if max <= 0 {
		return 0
	}
	return a.h.MPValue() / max
}

// X satisfies conditions.Actor.
func (a hostileStatActor) X() int { return a.h.X() }

// Y satisfies conditions.Actor.
func (a hostileStatActor) Y() int { return a.h.Y() }

// Z satisfies conditions.Actor.
func (a hostileStatActor) Z() int { return a.h.Z() }

// IsMoving satisfies conditions.Actor.
func (a hostileStatActor) IsMoving() bool { return a.h.Move().Moving() }

// IsRunning satisfies conditions.Actor. True unless this NPC was spawned in
// walk stance (aCis Walkers.java onCreated's setWalkOrRun(false) for its
// WALKING_NPCS id subset) — every other NPC's Npc.onSpawn calls
// setWalkOrRun(true) and the Java reference never toggles an NPC back
// (Creature's walk/run toggle is otherwise player-command driven only).
func (a hostileStatActor) IsRunning() bool { return a.h.Running() }

// IsRiding satisfies conditions.Actor. Always false: NPCs are never
// mounted, matching Creature.isRiding's un-overridden default.
func (a hostileStatActor) IsRiding() bool { return false }

// IsFlying satisfies conditions.Actor. Always false: no shipped NPC
// overrides Creature.isFlying's default.
func (a hostileStatActor) IsFlying() bool { return false }

// headingActor is the extra capability IsBehind/IsInFrontOf need beyond
// conditions.Actor's plain X/Y/Z: the reference position's own heading.
// conditionGate always tests an owner against itself
// (conditionGate.Test(actor, actor, nil)), so other is always this same
// hostileStatActor, which implements it via CurrentHeading below.
type headingActor interface{ CurrentHeading() int }

// CurrentHeading lets a hostileStatActor serve as the "other" side of its
// own IsBehind/IsInFrontOf check.
func (a hostileStatActor) CurrentHeading() int { return a.h.Heading() }

// IsBehind satisfies conditions.Actor: reports whether a is positioned
// behind other, using other's own facing (matching
// creature.ResolveBlowInput's identical behind/front check).
func (a hostileStatActor) IsBehind(other conditions.Actor) bool {
	h, ok := other.(headingActor)
	if !ok {
		return false
	}
	facing := location.OrientedLocation{
		Location: location.Location{X: other.X(), Y: other.Y(), Z: other.Z()},
		Heading:  h.CurrentHeading(),
	}
	return facing.IsBehind(location.Location{X: a.X(), Y: a.Y(), Z: a.Z()})
}

// IsInFrontOf satisfies conditions.Actor: reports whether a is positioned in
// front of other, using other's own facing.
func (a hostileStatActor) IsInFrontOf(other conditions.Actor) bool {
	h, ok := other.(headingActor)
	if !ok {
		return false
	}
	facing := location.OrientedLocation{
		Location: location.Location{X: other.X(), Y: other.Y(), Z: other.Z()},
		Heading:  h.CurrentHeading(),
	}
	return facing.IsInFrontOf(location.Location{X: a.X(), Y: a.Y(), Z: a.Z()})
}

// ActiveSkillLevel satisfies conditions.Actor.
func (a hostileStatActor) ActiveSkillLevel(id int) (int, bool) {
	return a.h.EffectList().ActiveBySkillID(id)
}

// ActiveEffectLevel satisfies conditions.Actor.
func (a hostileStatActor) ActiveEffectLevel(effectID int) (int, bool) {
	return a.h.EffectList().ActiveBySkillID(effectID)
}
