package hall

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/warrior"
)

// CrazyYeti is AzitWateringCrazyYeti: the enraged yeti of the Rainbow
// Springs hot springs game, a civilian that shouts its waking line as it
// appears and attacks the players and summons it sees, as a
// WarriorAggressive does.
func CrazyYeti() script.Script {
	base := warrior.AggressiveHooks()
	return script.Script{Dir: dir, Behavior: true, NPCs: []int32{35592}, Hooks: base.With(script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			e.NPC.Shout(script.NpcString(1010627))
			base.Created(s, e)
		},
	})}
}
