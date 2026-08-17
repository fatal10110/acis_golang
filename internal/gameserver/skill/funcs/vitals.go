package funcs

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/statbonus"
)

// MaxCpMul finalizes max CP from CON.
var MaxCpMul Func = func(effector stat.Actor, base, value float64) float64 {
	return value * statbonus.CONBonus[effector.CON()]
}

// MaxHpMul finalizes max HP from CON.
var MaxHpMul Func = func(effector stat.Actor, base, value float64) float64 {
	return value * statbonus.CONBonus[effector.CON()]
}

// MaxMpMul finalizes max MP from MEN.
var MaxMpMul Func = func(effector stat.Actor, base, value float64) float64 {
	return value * statbonus.MENBonus[effector.MEN()]
}

// RegenCpMul finalizes CP regen rate from CON and the level-scaling factor.
var RegenCpMul Func = func(effector stat.Actor, base, value float64) float64 {
	return value * statbonus.CONBonus[effector.CON()] * effector.LevelMod()
}

// RegenHpMul finalizes HP regen rate from CON and the level-scaling factor.
var RegenHpMul Func = func(effector stat.Actor, base, value float64) float64 {
	return value * statbonus.CONBonus[effector.CON()] * effector.LevelMod()
}

// RegenMpMul finalizes MP regen rate from MEN and the level-scaling factor.
var RegenMpMul Func = func(effector stat.Actor, base, value float64) float64 {
	return value * statbonus.MENBonus[effector.MEN()] * effector.LevelMod()
}
