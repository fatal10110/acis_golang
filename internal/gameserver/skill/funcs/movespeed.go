package funcs

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/statbonus"
)

// MoveSpeed finalizes run speed from DEX.
var MoveSpeed Func = func(effector stat.Actor, base, value float64) float64 {
	return value * statbonus.DEXBonus[effector.DEX()]
}
