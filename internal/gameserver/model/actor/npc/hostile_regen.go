package npc

import (
	"math"

	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// HPRegenRate returns this NPC's HP regeneration per tick
// (CreatureStatus.getRegenHp: calcStat(REGENERATE_HP_RATE, template base)).
func (h *Hostile) HPRegenRate() float64 {
	return h.calcStat(stat.RegenerateHPRate, h.Instance.Template.HPRegen)
}

// MPRegenRate returns this NPC's MP regeneration per tick
// (CreatureStatus.getRegenMp: calcStat(REGENERATE_MP_RATE, template base)).
func (h *Hostile) MPRegenRate() float64 {
	return h.calcStat(stat.RegenerateMPRate, h.Instance.Template.MPRegen)
}

// TickRegen applies one HP/MP regeneration step (CreatureStatus.
// doRegeneration: each resource short of its calculated max gains at least
// 1, then the resulting HP is broadcast to known observers) and is a no-op
// once this NPC has died.
func (h *Hostile) TickRegen() {
	if h.AlikeDead() {
		return
	}
	changed := false
	if h.HP() < h.MaxHPValue() {
		h.AddHP(math.Max(1, h.HPRegenRate()))
		changed = true
	}
	if h.MPValue() < h.MaxMPValue() {
		h.AddMP(math.Max(1, h.MPRegenRate()))
		changed = true
	}
	if !changed {
		return
	}
	if err := h.BroadcastStatus(); err != nil {
		h.log.Warn().Err(err).Msg("npc: regen status broadcast")
	}
}
