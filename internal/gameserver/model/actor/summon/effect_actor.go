package summon

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

var (
	_ effect.SummonActor = (*Actor)(nil)
	_ effect.CasterActor = (*Actor)(nil)
)

// OwnerObject returns the controlling player's world object.
func (a *Actor) OwnerObject() (world.Tracked, bool) {
	if a.owner == nil {
		return nil, false
	}
	return a.owner, true
}

// BroadcastStatus republishes the summon's vitals to its owner's pet window
// and to observers; see UpdateStatus.
func (a *Actor) BroadcastStatus() {
	a.UpdateStatus()
}

// AbortAll stops the summon's movement, attack and cast and sends it idle
// (see TryToIdle), then clears its target when resetTarget is set. None of
// it is client-visible beyond the stop broadcasts the controllers already
// send.
func (a *Actor) AbortAll(resetTarget bool) {
	if a.brain != nil {
		a.brain.AbortAll()
	}
	a.TryToIdle()
	if resetTarget {
		a.SetTarget(nil)
	}
}

// AbortForTeleport stops current actions without starting a new follow walk
// from the old position. Following resumes after the new position is set.
func (a *Actor) AbortForTeleport() {
	if a.brain != nil {
		a.brain.AbortAll()
	}
	a.idle()
	a.SetTarget(nil)
}

// OnTeleported restores owner following after the summon rejoins the world.
func (a *Actor) OnTeleported() {
	a.followOff.Store(false)
	a.goIdle()
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

// Afraid reports whether a held effect fears the summon.
func (a *Actor) Afraid() bool { return a.effects.IsAffected(effect.FlagFear) }

// FearImmune reports whether fear cannot take hold of the summon: siege
// summons shrug it off.
func (a *Actor) FearImmune() bool { return a.SiegeSummon() }

// FleeFrom makes running distance units directly away from effector the
// summon's current intention, the way a move request does. A summon that
// could not take AI actions before the effect in progress landed, which
// includes one already afraid, stays put; one that cannot move goes idle.
// Summons are always in run stance, so no stance change is sent; hungry-pet
// walk stance arrives with the feed tick (#2378).
func (a *Actor) FleeFrom(effector effect.Actor, distance int) {
	if effector == nil || effector.ObjectID() == a.ObjectID() || distance < 10 || a.brain == nil {
		return
	}
	if a.aiDeniedBeforeEffect() {
		return
	}
	if a.MovementDisabled() {
		a.TryToIdle()
		return
	}
	fromX, fromY, _ := effector.Position()
	a.brain.TryToMoveTo(a.Move().Position().FleeFrom(fromX, fromY, distance))
}

// aiDeniedBeforeEffect is DenyAIAction limited to effects whose on-start
// hook has completed; see effect.List.StartedAffected.
func (a *Actor) aiDeniedBeforeEffect() bool {
	a.stateMu.RLock()
	paralyzed := a.paralyzed
	a.stateMu.RUnlock()
	return a.AlikeDead() || paralyzed || a.Teleporting() || a.effects.StartedAffected(effect.AIDenyFlags)
}

// BluffExempt reports whether bluff cannot turn the summon: siege summons
// are exempt.
func (a *Actor) BluffExempt() bool { return a.SiegeSummon() }

// StopEffects removes every effect of type t the summon holds.
func (a *Actor) StopEffects(t effect.Type) { a.effects.StopByType(t) }

// StopSkillEffectsByID removes every effect skill id applied to the summon.
func (a *Actor) StopSkillEffectsByID(id modelskill.ID) { a.effects.StopBySkillID(id) }

// AddChanceTrigger registers a started chance-skill-trigger effect as one of
// the summon's chance procs.
func (a *Actor) AddChanceTrigger(e *effect.Effect) { a.effects.AddChanceTrigger(e) }

// RemoveChanceTrigger drops an exiting chance-skill-trigger effect from the
// summon's chance procs.
func (a *Actor) RemoveChanceTrigger(e *effect.Effect) { a.effects.RemoveChanceTrigger(e) }

// ValidLocation resolves a knockback destination against this summon's
// movement geodata, the same correction a player's landing gets.
func (a *Actor) ValidLocation(ox, oy, oz, tx, ty, tz int) location.Location {
	return a.movement.ValidLocation(ox, oy, oz, tx, ty, tz)
}

// FlyTo broadcasts a forced-flight animation without changing server position.
func (a *Actor) FlyTo(dest location.Location, flight modelskill.Flight) {
	a.emit(event.Flight{Dest: dest, Flight: flight})
}

// SetXYZ moves the summon immediately and reseeds its ordinary movement
// state so the next move starts from the forced landing.
func (a *Actor) SetXYZ(x, y, z int) {
	position := location.Location{X: x, Y: y, Z: z}
	a.movement.SetPosition(position)
	a.SyncPosition(position)
}

// BroadcastPosition sends the forced-location correction after a flight lands.
func (a *Actor) BroadcastPosition() {
	a.emit(event.PositionCorrected{})
}

// UpdateEffectIcons refreshes the summon's effect icons.
func (a *Actor) UpdateEffectIcons() { a.UpdateAbnormalEffect() }

// NotifyEffectWornOff does nothing: effect expiry messages go to players.
func (a *Actor) NotifyEffectWornOff(modelskill.ID, int) {}

// NotifyEffectDisappeared does nothing: effect expiry messages go to players.
func (a *Actor) NotifyEffectDisappeared(modelskill.ID, int) {}

// NotifyEffectAborted does nothing: effect expiry messages go to players.
func (a *Actor) NotifyEffectAborted(modelskill.ID, int) {}
