package funcs

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/statbonus"
)

// moveSpeed finalizes run speed from DEX.
type moveSpeed struct{ fixed }

var MoveSpeed = &moveSpeed{fixed{stat.RunSpeed}}

func (*moveSpeed) Calc(effector, effected, skill any, base, value float64) float64 {
	return value * statbonus.DEXBonus[actorOf(effector).DEX()]
}
