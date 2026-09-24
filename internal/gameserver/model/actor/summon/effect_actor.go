package summon

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

var _ effect.SummonActor = (*Actor)(nil)

// OwnerObject returns the controlling player's world object.
func (a *Actor) OwnerObject() (world.Tracked, bool) {
	if a.owner == nil {
		return nil, false
	}
	return a.owner, true
}

// AbortAll stops the summon's movement, attack and cast and sends it idle
// (see TryToIdle), then clears its target when resetTarget is set. None of
// it is client-visible beyond the stop broadcasts the controllers already
// send.
//
// ponytail: the idle is unconditional. An actor already disabled before the
// interrupting effect keeps its intention instead, but no command can give a
// disabled summon a new intention, so it already holds the one this sets.
func (a *Actor) AbortAll(resetTarget bool) {
	if a.brain != nil {
		a.brain.AbortAll()
	}
	a.TryToIdle()
	if resetTarget {
		a.SetTarget(nil)
	}
}

// StopMove stops the summon's movement.
func (a *Actor) StopMove() {
	if a.brain != nil {
		a.brain.StopMove()
	}
}

// ClearTarget clears the summon's target.
func (a *Actor) ClearTarget() { a.SetTarget(nil) }

// StopAttack stops the summon's attack and sends it idle; see AbortAll.
func (a *Actor) StopAttack() {
	if a.brain != nil {
		a.brain.StopAttack()
	}
	a.TryToIdle()
}

// Afraid reports false: summon fear state is not modeled yet.
func (a *Actor) Afraid() bool { return false }

// FearImmune reports false: fear immunity is not modeled yet.
func (a *Actor) FearImmune() bool { return false }

// FleeFrom reports false: fleeing movement is not modeled yet.
func (a *Actor) FleeFrom(effect.Actor, int) bool { return false }

// BluffExempt reports false: summons are never exempt from bluff.
func (a *Actor) BluffExempt() bool { return false }

// StopEffects does nothing yet: stopping effects by type is not wired.
func (a *Actor) StopEffects(effect.Type) {}

// StopSkillEffectsByID does nothing yet: stopping effects by skill is not
// wired.
func (a *Actor) StopSkillEffectsByID(modelskill.ID) {}

// AddChanceTrigger does nothing yet: chance skill triggers are not wired.
func (a *Actor) AddChanceTrigger(*effect.Effect) {}

// RemoveChanceTrigger does nothing yet: chance skill triggers are not wired.
func (a *Actor) RemoveChanceTrigger(*effect.Effect) {}

// ValidLocation returns the destination unchanged: summons are never knocked
// back, so no geodata correction applies.
func (a *Actor) ValidLocation(_, _, _, tx, ty, tz int) location.Location {
	return location.Location{X: tx, Y: ty, Z: tz}
}

// FlyTo does nothing: summons are never knocked back.
func (a *Actor) FlyTo(location.Location, modelskill.Flight) {}

// SetXYZ does nothing: summons are never knocked back.
func (a *Actor) SetXYZ(int, int, int) {}

// BroadcastPosition does nothing: summons are never knocked back.
func (a *Actor) BroadcastPosition() {}

// UpdateEffectIcons refreshes the summon's effect icons.
func (a *Actor) UpdateEffectIcons() { a.UpdateAbnormalEffect() }

// NotifyEffectWornOff does nothing: effect expiry messages go to players.
func (a *Actor) NotifyEffectWornOff(modelskill.ID, int) {}

// NotifyEffectDisappeared does nothing: effect expiry messages go to players.
func (a *Actor) NotifyEffectDisappeared(modelskill.ID, int) {}

// NotifyEffectAborted does nothing: effect expiry messages go to players.
func (a *Actor) NotifyEffectAborted(modelskill.ID, int) {}
