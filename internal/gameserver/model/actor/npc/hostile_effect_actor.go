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

// FearImmune reports false: fear immunity is not modeled yet.
func (h *Hostile) FearImmune() bool { return false }

// FleeFrom reports false: fleeing movement is not modeled yet.
func (h *Hostile) FleeFrom(effect.Actor, int) bool { return false }

// BluffExempt reports false: no NPC kind is exempt from bluff yet.
func (h *Hostile) BluffExempt() bool { return false }

// StopEffects does nothing yet: stopping effects by type is not wired.
func (h *Hostile) StopEffects(effect.Type) {}

// StopSkillEffectsByID does nothing yet: stopping effects by skill is not
// wired.
func (h *Hostile) StopSkillEffectsByID(modelskill.ID) {}

// AddChanceTrigger does nothing yet: chance skill triggers are not wired.
func (h *Hostile) AddChanceTrigger(*effect.Effect) {}

// RemoveChanceTrigger does nothing yet: chance skill triggers are not wired.
func (h *Hostile) RemoveChanceTrigger(*effect.Effect) {}

// UpdateEffectIcons does nothing: NPCs show no effect icons.
func (h *Hostile) UpdateEffectIcons() {}

// NotifyEffectWornOff does nothing: effect expiry messages go to players.
func (h *Hostile) NotifyEffectWornOff(modelskill.ID, int) {}

// NotifyEffectDisappeared does nothing: effect expiry messages go to players.
func (h *Hostile) NotifyEffectDisappeared(modelskill.ID, int) {}

// NotifyEffectAborted does nothing: effect expiry messages go to players.
func (h *Hostile) NotifyEffectAborted(modelskill.ID, int) {}
