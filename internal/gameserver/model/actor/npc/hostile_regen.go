package npc

import (
	"math"

	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// HPRegenRate returns this NPC's HP regeneration per tick: the template
// base, scaled by the raid HP regen multiplier while raid related, finalized
// through the stat calculator.
func (h *Hostile) HPRegenRate() float64 {
	return h.calcStat(stat.RegenerateHPRate, h.Instance.Template.HPRegen*h.raidBaseMultipliers().HPRegen)
}

// MPRegenRate returns this NPC's MP regeneration per tick: the template
// base, scaled by the raid MP regen multiplier while raid related, finalized
// through the stat calculator.
func (h *Hostile) MPRegenRate() float64 {
	return h.calcStat(stat.RegenerateMPRate, h.Instance.Template.MPRegen*h.raidBaseMultipliers().MPRegen)
}

// TickRegen applies one HP/MP regeneration step (CreatureStatus.
// doRegeneration: each resource short of its calculated max gains at least
// 1, then the resulting HP is broadcast to known observers) and is a no-op
// once this NPC has died. A kill landing from another queue after the
// liveness check still stops the tick: neither write touches a corpse, and
// a tick that wrote nothing reports nothing.
func (h *Hostile) TickRegen() {
	if h.AlikeDead() {
		return
	}
	changed := false
	if h.HP() < h.MaxHPValue() {
		changed = h.health.Add(math.Max(1, h.HPRegenRate()), h.MaxHPValue()) > 0
	}
	if h.MPValue() < h.MaxMPValue() {
		changed = h.addMP(math.Max(1, h.MPRegenRate())) > 0 || changed
	}
	if !changed {
		return
	}
	h.BroadcastStatus()
}
