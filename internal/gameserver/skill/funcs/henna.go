package funcs

import "github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"

// hennaFunc adds a player's applied-henna bonus for s (one of the six base
// attributes); a non-player effector is unaffected.
func hennaFunc(s stat.Stat) Func {
	return func(effector stat.Actor, base, value float64) float64 {
		if p, ok := effector.(stat.PlayerActor); ok {
			return value + p.HennaBonus(s)
		}
		return value
	}
}

// HennaSTR, HennaCON, HennaDEX, HennaINT, HennaWIT and HennaMEN are the
// shared instances every player's calculation chain attaches for its
// corresponding base attribute, adding whatever bonus its applied hennas
// grant.
var (
	HennaSTR = hennaFunc(stat.StatSTR)
	HennaCON = hennaFunc(stat.StatCON)
	HennaDEX = hennaFunc(stat.StatDEX)
	HennaINT = hennaFunc(stat.StatINT)
	HennaWIT = hennaFunc(stat.StatWIT)
	HennaMEN = hennaFunc(stat.StatMEN)
)
