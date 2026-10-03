package summon

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
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
	owner := a.currentOwner()
	if owner == nil {
		return nil, false
	}
	return owner, true
}

// RandomConfusionTarget returns a random confusion target known within
// radius units of the summon, its owner included; see
// npc.RandomConfusionTarget.
func (a *Actor) RandomConfusionTarget(radius int) (world.Tracked, bool) {
	return npc.RandomConfusionTarget(a.world, a, radius, a.Roll)
}

// BroadcastStatus republishes the summon's vitals after a vitals setter:
// the players targeting it may get its current HP, then UpdateStatus
// refreshes its owner's pet window and observers.
func (a *Actor) BroadcastStatus() {
	a.emit(event.HPChanged{})
	a.UpdateStatus()
}

// AbortAll stops the summon's movement, attack and cast and sends it idle
// (see TryToIdle), then clears its target when resetTarget is set. It is the
// start of an effect taking hold, so a cast it stops that had replaced an
// attack resumes it before that effect's flags are raised: a summon still in
// reach swings once more (see effectHeld and ai.Summon.AbortAllForEffect).
func (a *Actor) AbortAll(resetTarget bool) {
	if a.brain != nil {
		a.effectAborts.Add(1)
		a.brain.AbortAllForEffect()
		a.effectAborts.Add(-1)
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
	// A flee asked for mid-swing (the swing a fear's abort resumes) waits
	// for it, and by then the fear holds the summon: it never runs, and
	// the swing's end sends it idle (see FinishedAttack).
	if a.brain.AttackingNow() {
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
	return a.AlikeDead() || a.paralyzedLock() || a.Teleporting() || a.effects.StartedAffected(effect.AIDenyFlags)
}

// effectHeld reports whether a held effect carrying any flag in mask
// affects the summon. While AbortAll runs, only effects whose on-start hook
// has completed count: the effect calling it raises its own flags once that
// hook returns, so the attack a stopped cast resumes is judged as the summon
// stood before that effect landed (see effect.List.StartedAffected).
func (a *Actor) effectHeld(mask effect.Flag) bool {
	if a.effectAborts.Load() > 0 {
		return a.effects.StartedAffected(mask)
	}
	return a.effects.IsAffected(mask)
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
	a.relocate(position, true)
}

// BroadcastPosition sends the forced-location correction after a flight lands.
func (a *Actor) BroadcastPosition() {
	a.emit(event.PositionCorrected{})
}

// UpdateEffectIcons reports that the summon's effect icons changed: its
// owner's party, or its owner alone, is shown them.
func (a *Actor) UpdateEffectIcons() { a.emit(event.EffectIconsChanged{}) }

// NotifyEffectWornOff does nothing: effect expiry messages go to players.
func (a *Actor) NotifyEffectWornOff(modelskill.ID, int) {}

// NotifyEffectDisappeared does nothing: effect expiry messages go to players.
func (a *Actor) NotifyEffectDisappeared(modelskill.ID, int) {}

// NotifyEffectAborted does nothing: effect expiry messages go to players.
func (a *Actor) NotifyEffectAborted(modelskill.ID, int) {}

// StopCharmOfLuck runs when a Charm of Luck ends on the summon: its
// appearance is refreshed.
func (a *Actor) StopCharmOfLuck(*effect.Effect) { a.UpdateAbnormalEffect() }

// StopPhoenixBlessing runs when a Phoenix Blessing ends on the summon: its
// appearance is refreshed.
func (a *Actor) StopPhoenixBlessing(*effect.Effect) { a.UpdateAbnormalEffect() }

// StopProtectionBlessing runs when a Blessing of Protection loses its stack
// group's head on the summon: its appearance is refreshed.
func (a *Actor) StopProtectionBlessing(*effect.Effect) { a.UpdateAbnormalEffect() }

// NotifyEffectFelt does nothing: stack-change messages go to players.
func (a *Actor) NotifyEffectFelt(modelskill.ID, int) {}
