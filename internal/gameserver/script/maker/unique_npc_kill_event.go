package maker

import "github.com/fatal10110/acis_golang/internal/gameserver/script"

// UniqueNpcKillEvent is the maker whose unique NPC (the alias its
// unique_npc memo names) leaving acts as its event memo says: 0 runs the
// maker's own script event 1000, deleting the group; 1 runs 1001 on the
// npcmaker its maker_name memo names. The default maker's handling
// follows for every NPC leaving.
func UniqueNpcKillEvent() script.Maker {
	base := defaultHooks()
	return script.Maker{MakerHooks: base.With(script.MakerHooks{
		OnNPCDeleted: func(m *script.Maker, e script.MakerNPC) {
			if uniqueSpawn(e.Group, e.Spawn) {
				switch memoInt(e.Group, "event") {
				case 0:
					m.ScriptEvent(m, script.MakerEvent{Group: e.Group, Name: "1000"})
				case 1:
					sendNamed(e.Group, "maker_name", "1001", 0, 0)
				}
			}
			base.NPCDeleted(m, e)
		},
	})}
}
