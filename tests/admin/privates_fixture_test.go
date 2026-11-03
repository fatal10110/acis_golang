package admin

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// withPartyLeader binds to the monster id a behavior that calls its
// privates as it is created, as a party leader's behavior does: a spawn
// itself creates no privates.
func withPartyLeader(id int32) gameservertest.Option {
	const path = "ai.FixturePartyLeader"
	return gameservertest.WithNPCScripts(
		map[int32]script.NPCKind{id: script.KindHostile},
		[]script.Listing{{Path: path}},
		script.Catalog{path: func() script.Script {
			return script.Script{Behavior: true, NPCs: []int32{id}, Hooks: script.Hooks{
				OnCreated: func(s *script.Script, e script.Created) { defaultnpc.CreatePrivates(s, e.NPC) },
			}}
		}},
	)
}
