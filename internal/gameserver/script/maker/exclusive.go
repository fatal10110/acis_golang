package maker

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// ExclusiveSpawnNormal is the maker whose unique NPC (the alias its
// unique_npc memo names) keeps every other NPC of the group out: while the
// unique NPC is in (memo i_ai0 1), any other NPC entering is deleted. The
// unique NPC entering runs script event 1000 with int1 1 on the npcmaker
// its maker_name memo names; leaving, 1001 there, then the default maker's
// handling. Another NPC leaving while the unique one is out respawns after
// its spawn's delay while the group is under its maximum. Every script
// event sets i_ai0 to int1 before the default maker's handling.
func ExclusiveSpawnNormal() script.Maker {
	uniqueIn := &intMemo{key: "i_ai0"}
	base := defaultHooks()
	return script.Maker{MakerHooks: base.With(script.MakerHooks{
		OnStart: func(m *script.Maker, e script.MakerStart) {
			uniqueIn.set(0)
			base.Start(m, e)
		},
		OnNPCCreated: func(_ *script.Maker, e script.MakerNPC) {
			switch {
			case uniqueSpawn(e.Group, e.Spawn):
				uniqueIn.set(1)
				sendNamed(e.Group, "maker_name", "1000", 1, 0)
			case uniqueIn.get() == 1:
				e.NPC.Delete()
			}
		},
		OnNPCDeleted: func(m *script.Maker, e script.MakerNPC) {
			switch {
			case uniqueSpawn(e.Group, e.Spawn):
				uniqueIn.set(0)
				sendNamed(e.Group, "maker_name", "1001", 0, 0)
				base.NPCDeleted(m, e)
			case uniqueIn.get() == 0:
				respawnUnderMaximum(e)
			}
		},
		OnScriptEvent: func(m *script.Maker, e script.MakerEvent) {
			uniqueIn.set(e.Int1)
			base.ScriptEvent(m, e)
		},
	})}
}

// ExclusiveDespawnNormal is the maker whose unique NPC (the alias its
// unique_npc memo names) lets every other NPC of the group in: while it is
// out (memo i_ai0 0), any other NPC entering is deleted. The unique NPC
// entering runs script event 1001 with int1 1 on the npcmakers its
// maker_name1 and maker_name2 memos name; leaving, 1000 on the first and
// 10025 on the second, then the default maker's handling. Another NPC
// leaving while the unique one is in respawns after its spawn's delay
// while the group is under its maximum.
//
// Script event 10025 counts (memo i_ai1); the despawn_count-th one runs
// 1000 on the npcmaker maker_name2 names and, when there is one, sets
// i_ai0 to int1 and the count back to 0. Any other script event sets i_ai0
// to int1 and nothing more.
func ExclusiveDespawnNormal() script.Maker {
	uniqueIn := &intMemo{key: "i_ai0"}
	despawns := &intMemo{key: "i_ai1"}
	base := defaultHooks()
	return script.Maker{MakerHooks: base.With(script.MakerHooks{
		OnStart: func(m *script.Maker, e script.MakerStart) {
			uniqueIn.set(0)
			despawns.set(0)
			base.Start(m, e)
		},
		OnNPCCreated: func(_ *script.Maker, e script.MakerNPC) {
			switch {
			case uniqueSpawn(e.Group, e.Spawn):
				uniqueIn.set(1)
				sendNamed(e.Group, "maker_name1", "1001", 1, 0)
				sendNamed(e.Group, "maker_name2", "1001", 1, 0)
			case uniqueIn.get() == 0:
				e.NPC.Delete()
			}
		},
		OnNPCDeleted: func(m *script.Maker, e script.MakerNPC) {
			switch {
			case uniqueSpawn(e.Group, e.Spawn):
				uniqueIn.set(0)
				sendNamed(e.Group, "maker_name1", "1000", 0, 0)
				sendNamed(e.Group, "maker_name2", "10025", 0, 0)
				base.NPCDeleted(m, e)
			case uniqueIn.get() == 1:
				respawnUnderMaximum(e)
			}
		},
		OnScriptEvent: func(_ *script.Maker, e script.MakerEvent) {
			if !strings.EqualFold(e.Name, "10025") {
				uniqueIn.set(e.Int1)
				return
			}
			n := despawns.get() + 1
			despawns.set(n)
			if n == memoInt(e.Group, "despawn_count") && sendNamed(e.Group, "maker_name2", "1000", 0, 0) {
				uniqueIn.set(e.Int1)
				despawns.set(0)
			}
		},
	})}
}

// uniqueSpawn reports whether the NPCs of ms have the template alias g's
// unique_npc memo names, ignoring case; never when the memo names none.
func uniqueSpawn(g spawn.Group, ms spawn.GroupSpawn) bool {
	alias, ok := g.Maker().AIParams["unique_npc"]
	return ok && strings.EqualFold(ms.Alias(), alias)
}

// sendNamed runs script event name, with int1 and int2, on the npcmaker
// g's memo key names, and reports whether there is one; nothing runs when
// the memo names none.
func sendNamed(g spawn.Group, key, name string, int1, int2 int) bool {
	target, ok := g.Maker().AIParams[key]
	return ok && g.SendMakerEvent(target, name, int1, int2)
}

// respawnUnderMaximum schedules the NPC's respawn after its spawn's
// randomized delay while the group is under its maximum.
func respawnUnderMaximum(e script.MakerNPC) {
	if e.Group.Maker().MaximumNPCs-e.Group.Alive() > 0 {
		e.NPC.ScheduleRespawn(spawn.CalculateRespawnDelay(e.Spawn.Entry()))
	}
}
