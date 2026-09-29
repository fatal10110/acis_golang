package cast

import (
	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
)

// summonStatusRefresher is a summon whose current status can be republished
// to its owner's pet window and to the players who see it.
type summonStatusRefresher interface {
	UpdateStatus()
}

// RefreshSummonTargets republishes the status of every summon among a
// player cast's affected targets. A player's hit runs it after its charge
// step and before the skill's effects land, so the owner's pet window and
// the observers are refreshed ahead of whatever the skill then changes.
// Only a player caster's hit does this; an NPC's or a summon's does not.
func RefreshSummonTargets(affected []skilltarget.Actor) {
	for _, target := range affected {
		if target == nil || target.Kind() != actor.KindSummon {
			continue
		}
		if summon, ok := target.(summonStatusRefresher); ok {
			summon.UpdateStatus()
		}
	}
}
