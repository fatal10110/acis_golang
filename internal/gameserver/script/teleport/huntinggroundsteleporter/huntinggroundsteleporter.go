// Package huntinggroundsteleporter is the Seven Signs priests' teleport to
// the hunting grounds: a player who signed up for a cabal gets the
// priest's teleport window, one who did not the refusal of the priest's
// side.
package huntinggroundsteleporter

import (
	"slices"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/travel"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
)

// priests are the priests of both sides.
func priests() []int32 {
	return []int32{
		31078,
		31079,
		31080,
		31081,
		31082,
		31083,
		31084,
		31085,
		31086,
		31087,
		31088,
		31089,
		31090,
		31091,
		31168,
		31169,
		31692,
		31693,
		31694,
		31695,
		31997,
		31998,
	}
}

// dawnNpcs are the priests of the Lords of Dawn.
func dawnNpcs() []int32 {
	return []int32{
		31078,
		31079,
		31080,
		31081,
		31082,
		31083,
		31084,
		31168,
		31692,
		31694,
		31997,
	}
}

// New returns the teleporter.
func New() script.Script {
	return script.Script{
		Dir:   "teleport",
		Bind:  script.Bindings{script.EventTalked: priests()},
		Hooks: script.Hooks{OnTalk: onTalk},
	}
}

func onTalk(s *script.Script, e script.Talk) string {
	playerCabal := s.PlayerCabal(e.Player.ObjectID())
	if playerCabal == sevensigns.NoCabal {
		if slices.Contains(dawnNpcs(), e.NPC.NpcID()) {
			return "dawn_tele-no.htm"
		}
		return "dusk_tele-no.htm"
	}

	e.NPC.ShowTeleportWindow(e.Player, travel.KindStandard)
	return ""
}
