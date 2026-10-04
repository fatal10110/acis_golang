package summon

import (
	"math"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// HPRegenRate returns a's HP regeneration per tick: its npc template's base
// through its live stat calculator, scaled by a pet's weight-penalty band.
func (a *Actor) HPRegenRate() float64 {
	return a.calcStat(stat.RegenerateHPRate, a.combatStats().HPRegen) * weightPenaltyRegen[a.weightPenalty.Load()]
}

// MPRegenRate returns a's MP regeneration per tick: its npc template's base
// through its live stat calculator, scaled by a pet's weight-penalty band.
func (a *Actor) MPRegenRate() float64 {
	return a.calcStat(stat.RegenerateMPRate, a.combatStats().MPRegen) * weightPenaltyRegen[a.weightPenalty.Load()]
}

// TickRegen applies one HP/MP regeneration step: each resource short of its
// maximum gains its rate, at least 1, and a change republishes a's status
// once for both resources. A dead summon does not regenerate, whether it
// died here or was restored as a corpse. Either way the task is then
// settled, so a tick that fills the summon, or finds it dead, stops it.
func (a *Actor) TickRegen() {
	if a.Dead() {
		a.SettleRegen()
		return
	}
	changed := a.addHP(math.Max(1, a.HPRegenRate())) > 0
	changed = a.addMP(math.Max(1, a.MPRegenRate())) > 0 || changed
	if changed {
		a.BroadcastStatus()
		return
	}
	a.SettleRegen()
}

// Regen returns a's regeneration phase, which the regeneration sweep
// polls and claims.
func (a *Actor) Regen() *creature.Regen { return &a.regen }

// SettleRegen arms a's regeneration task when it is spawned, alive and
// short of HP or MP, its first tick one period from now, and disarms
// it otherwise (CreatureStatus.setHp/setMp's start and stop).
func (a *Actor) SettleRegen() {
	a.regen.Settle(a.Queue(), a.regenShort)
}

// regenShort reports whether a regenerates: spawned, not dead, and below
// its maximum HP or MP.
func (a *Actor) regenShort() bool {
	return a.Visible() && !a.Dead() && (a.HP() < a.MaxHPValue() || a.MPValue() < a.MaxMPValue())
}
