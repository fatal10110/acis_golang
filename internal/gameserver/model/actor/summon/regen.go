package summon

import (
	"math"

	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// HPRegenRate returns a's HP regeneration per tick: its npc template's base
// through its live stat calculator.
func (a *Actor) HPRegenRate() float64 {
	return a.calcStat(stat.RegenerateHPRate, a.combatStats().HPRegen)
}

// MPRegenRate returns a's MP regeneration per tick: its npc template's base
// through its live stat calculator.
func (a *Actor) MPRegenRate() float64 {
	return a.calcStat(stat.RegenerateMPRate, a.combatStats().MPRegen)
}

// TickRegen applies one HP/MP regeneration step: each resource short of its
// maximum gains its rate, at least 1, and a change republishes a's status
// once for both resources. A dead summon does not regenerate, whether it
// died here or was restored as a corpse.
func (a *Actor) TickRegen() {
	if a.Dead() {
		return
	}
	changed := a.addHP(math.Max(1, a.HPRegenRate())) > 0
	changed = a.addMP(math.Max(1, a.MPRegenRate())) > 0 || changed
	if changed {
		a.BroadcastStatus()
	}
}
