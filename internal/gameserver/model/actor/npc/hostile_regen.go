package npc

import (
	"math"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
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
// a tick that wrote nothing reports nothing. Either way the task is then
// settled, so a tick that fills the NPC, or finds it dead, stops it.
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
// it otherwise (CreatureStatus.setHp/setMp's start and stop).
func (h *Hostile) SettleRegen() {
	h.regen.Settle(h.Queue(), h.regenShort)
}

// regenShort reports whether this NPC regenerates: spawned (on the grid or
// off it mid-relocation), not dead, and below its maximum HP or MP.
func (h *Hostile) regenShort() bool {
	return h.Spawned() && !h.Dead() && (h.HP() < h.MaxHPValue() || h.MPValue() < h.MaxMPValue())
}
