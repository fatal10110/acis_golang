package npc

import (
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

var _ effect.NPCActor = (*Hostile)(nil)

// AbortAll stops the NPC's movement, attack and cast without sending it
// idle: while the effect that asked for the abort holds, the think loop
// declines to act on its intentions. An NPC keeps no selected target apart from its
// intentions, so resetTarget has nothing to clear.
func (h *Hostile) AbortAll(bool) {
	h.brain.AbortAll()
}

// StopMove stops the NPC's movement.
func (h *Hostile) StopMove() { h.move.Stop() }

// TryToIdle does nothing: only a player or summon is sent idle by an effect.
func (h *Hostile) TryToIdle() {}

// ClearTarget does nothing: an NPC keeps no selected target apart from its
// intentions, and clearing one is silent.
func (h *Hostile) ClearTarget() {}

// StopAttack stops the NPC's attack cycle; its intentions stay.
func (h *Hostile) StopAttack() { h.brain.StopAttack() }

// FearImmune reports false: only folk, siege flags and siege summons shrug
// off fear, and none of them is a hostile NPC.
func (h *Hostile) FearImmune() bool { return false }

// FleeFrom runs the NPC distance units directly away from effector, in run
// stance, on every call. The walk is geodata-routed from the NPC's current
// position; an NPC that cannot move keeps its place. It bypasses the think
// loop, which a fear already keeps from acting.
func (h *Hostile) FleeFrom(effector effect.Actor, distance int) {
	if effector == nil || effector.ObjectID() == h.ObjectID() || distance < 10 {
		return
	}
	h.ForceRunStance()
	if h.MovementDisabled() {
		return
	}
	fromX, fromY, _ := effector.Position()
	_, _ = h.move.MoveToLocation(h.Move().Position().FleeFrom(fromX, fromY, distance))
}

// BluffExempt reports false: the kinds bluff skips are folk, siege summons
// and raid-related NPCs, and the bluff hook checks the last itself.
func (h *Hostile) BluffExempt() bool { return false }

// StopEffects removes every effect of type t the NPC holds.
func (h *Hostile) StopEffects(t effect.Type) { h.EffectList().StopByType(t) }

// StopSkillEffectsByID removes every effect skill id applied to the NPC.
func (h *Hostile) StopSkillEffectsByID(id modelskill.ID) { h.EffectList().StopBySkillID(id) }

// AddChanceTrigger registers a started chance-skill-trigger effect as one of
// the NPC's chance procs.
func (h *Hostile) AddChanceTrigger(e *effect.Effect) { h.EffectList().AddChanceTrigger(e) }

// RemoveChanceTrigger drops an exiting chance-skill-trigger effect from the
// NPC's chance procs.
func (h *Hostile) RemoveChanceTrigger(e *effect.Effect) { h.EffectList().RemoveChanceTrigger(e) }

// UpdateEffectIcons does nothing: NPCs show no effect icons.
func (h *Hostile) UpdateEffectIcons() {}

// NotifyEffectWornOff does nothing: effect expiry messages go to players.
func (h *Hostile) NotifyEffectWornOff(modelskill.ID, int) {}

// NotifyEffectDisappeared does nothing: effect expiry messages go to players.
func (h *Hostile) NotifyEffectDisappeared(modelskill.ID, int) {}

// NotifyEffectAborted does nothing: effect expiry messages go to players.
func (h *Hostile) NotifyEffectAborted(modelskill.ID, int) {}
