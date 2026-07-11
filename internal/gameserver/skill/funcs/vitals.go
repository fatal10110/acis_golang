package funcs

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/statbonus"
)

// maxCpMul finalizes max CP from CON.
type maxCpMul struct{ fixed }

var MaxCpMul = maxCpMul{fixed{stat.MaxCP}}

func (maxCpMul) Calc(effector, effected, skill any, base, value float64) float64 {
	return value * statbonus.CONBonus[actorOf(effector).CON()]
}

// maxHpMul finalizes max HP from CON.
type maxHpMul struct{ fixed }

var MaxHpMul = maxHpMul{fixed{stat.MaxHP}}

func (maxHpMul) Calc(effector, effected, skill any, base, value float64) float64 {
	return value * statbonus.CONBonus[actorOf(effector).CON()]
}

// maxMpMul finalizes max MP from MEN.
type maxMpMul struct{ fixed }

var MaxMpMul = maxMpMul{fixed{stat.MaxMP}}

func (maxMpMul) Calc(effector, effected, skill any, base, value float64) float64 {
	return value * statbonus.MENBonus[actorOf(effector).MEN()]
}

// regenCpMul finalizes CP regen rate from CON and the level-scaling factor.
type regenCpMul struct{ fixed }

var RegenCpMul = regenCpMul{fixed{stat.RegenerateCPRate}}

func (regenCpMul) Calc(effector, effected, skill any, base, value float64) float64 {
	a := actorOf(effector)
	return value * statbonus.CONBonus[a.CON()] * a.LevelMod()
}

// regenHpMul finalizes HP regen rate from CON and the level-scaling factor.
type regenHpMul struct{ fixed }

var RegenHpMul = regenHpMul{fixed{stat.RegenerateHPRate}}

func (regenHpMul) Calc(effector, effected, skill any, base, value float64) float64 {
	a := actorOf(effector)
	return value * statbonus.CONBonus[a.CON()] * a.LevelMod()
}

// regenMpMul finalizes MP regen rate from MEN and the level-scaling factor.
type regenMpMul struct{ fixed }

var RegenMpMul = regenMpMul{fixed{stat.RegenerateMPRate}}

func (regenMpMul) Calc(effector, effected, skill any, base, value float64) float64 {
	a := actorOf(effector)
	return value * statbonus.MENBonus[a.MEN()] * a.LevelMod()
}
