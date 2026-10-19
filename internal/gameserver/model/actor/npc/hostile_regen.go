package npc

import (
	"math"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// HPRegenRate returns this NPC's HP regeneration per tick: the template
// base, scaled by the raid HP regen multiplier while raid related and by
// the configured one otherwise, finalized through the stat calculator.
func (h *Hostile) HPRegenRate() float64 {
	hp, _ := h.regenBase()
	return h.calcStat(stat.RegenerateHPRate, h.Instance.Template.HPRegen*hp)
}

// MPRegenRate returns this NPC's MP regeneration per tick: the template
// base, scaled by the raid MP regen multiplier while raid related and by
// the configured one otherwise, finalized through the stat calculator.
func (h *Hostile) MPRegenRate() float64 {
	_, mp := h.regenBase()
	return h.calcStat(stat.RegenerateMPRate, h.Instance.Template.MPRegen*mp)
}

// regenBase returns the multipliers h's base HP and MP regeneration take:
// the raid ones while h is raid related, in place of the configured ones
// it takes otherwise.
func (h *Hostile) regenBase() (hp, mp float64) {
	if h.RaidRelated() {
		m := h.raidBaseMultipliers()
		return m.HPRegen, m.MPRegen
	}
	m := regenInForce(h.regenSettings)
	return m.HP, m.MP
}

// TickRegen applies one HP/MP regeneration step (each resource short of its
// calculated max gains at least 1, then the resulting HP is broadcast to
// known observers) and is a no-op once this NPC has died. A kill landing
// from another queue after the liveness check still stops the tick: neither
// write touches a corpse, and a tick that wrote nothing reports nothing.
// Either way the task is then settled, so a tick that fills the NPC, or
// finds it dead, stops it.
func (h *Hostile) TickRegen() {
	if h.AlikeDead() {
		h.SettleRegen()
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
		h.SettleRegen()
		return
	}
	h.BroadcastStatus()
}

// Regen returns this NPC's regeneration phase, which the regeneration sweep
// polls and claims.
func (h *Hostile) Regen() *creature.Regen { return &h.regen }

// SettleRegen arms this NPC's regeneration task when it is spawned, alive
// and short of HP or MP, its first tick one period from now, and disarms
// it otherwise; every HP or MP change settles it so.
func (h *Hostile) SettleRegen() {
	h.regen.Settle(h.Queue(), h.regenShort)
}

// regenShort reports whether this NPC regenerates: spawned (on the grid or
// off it mid-relocation), not dead, and below its maximum HP or MP.
func (h *Hostile) regenShort() bool {
	return h.Spawned() && !h.Dead() && (h.HP() < h.MaxHPValue() || h.MPValue() < h.MaxMPValue())
}
