package npc

import (
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

var _ effect.NPCActor = (*Hostile)(nil)

// AbortAll does nothing yet: aborting every in-progress action is not wired.
func (h *Hostile) AbortAll(bool) {}

// StopMove does nothing yet: effect-driven movement stops are not wired.
func (h *Hostile) StopMove() {}

// TryToIdle does nothing yet: effect-driven NPC idling is not wired.
func (h *Hostile) TryToIdle() {}

// ClearTarget does nothing yet: effect-driven target clearing is not wired.
func (h *Hostile) ClearTarget() {}

// StopAttack does nothing yet: effect-driven attack stops are not wired.
func (h *Hostile) StopAttack() {}

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
