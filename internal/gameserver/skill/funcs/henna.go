package funcs

import "github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"

// henna adds a player's applied-henna bonus for one of the six base
// attributes; a non-player effector is unaffected.
type henna struct{ fixed }

func (h henna) Calc(effector, effected, skill any, base, value float64) float64 {
	if p, ok := effector.(PlayerActor); ok {
		return value + p.HennaBonus(h.s)
	}
	return value
}

// HennaSTR, HennaCON, HennaDEX, HennaINT, HennaWIT and HennaMEN are the
// shared instances every player's calculation chain attaches for its
// corresponding base attribute, adding whatever bonus its applied hennas
// grant.
var (
	HennaSTR = henna{fixed{stat.StatSTR}}
	HennaCON = henna{fixed{stat.StatCON}}
	HennaDEX = henna{fixed{stat.StatDEX}}
	HennaINT = henna{fixed{stat.StatINT}}
	HennaWIT = henna{fixed{stat.StatWIT}}
	HennaMEN = henna{fixed{stat.StatMEN}}
)
