package maker

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// ManageTeleportDungeon is the maker of a dimensional rift room. Its
// manager_npc_name memo names, by template alias, the room's manager; every
// other spawn of the room is an invader, kept out of the world while the
// room is closed (its i_ai0 memo at 0, as it starts). At start it places the
// manager. Script event 1 opens a closed room and brings each invader spawn
// whose total still fits the maximum; 0 closes the room and deletes the
// invaders; 2 tells the managers 2; 3 closes the room, tells the managers 3
// and deletes the invaders. The default maker's script events follow. An
// NPC leaving respawns only while the room is open.
func ManageTeleportDungeon() script.Maker {
	open := &intMemo{key: "i_ai0"}
	base := defaultHooks()
	return script.Maker{MakerHooks: base.With(script.MakerHooks{
		OnStart: func(_ *script.Maker, e script.MakerStart) {
			g := e.Group
			var def0 spawn.GroupSpawn
			if alias, ok := g.Maker().AIParams["manager_npc_name"]; ok {
				def0 = g.FindSpawn(alias)
			}
			if def0 != nil && g.Maker().MaximumNPCs-g.Alive() > 0 {
				// The bound shrinks with every NPC placed.
				for i := 0; i < def0.Total()-def0.Spawned(); i++ {
					def0.Spawn()
				}
			}
			open.set(0)
		},
		OnScriptEvent: func(m *script.Maker, e script.MakerEvent) {
			g := e.Group
			manager := managerOf(g)
			switch e.Name {
			case "0":
				open.set(0)
				for _, ms := range g.Spawns() {
					if !manager.names(ms.Alias()) {
						ms.DeleteNPCs()
					}
				}
			case "1":
				if open.get() == 0 {
					open.set(1)
					for _, ms := range g.Spawns() {
						if manager.names(ms.Alias()) {
							continue
						}
						if g.Maker().MaximumNPCs-(g.Alive()+ms.Total()) >= 0 {
							for range ms.Total() {
								if ms.Decayed() > 0 {
									ms.Respawn()
								} else {
									ms.Spawn()
								}
							}
						}
					}
				}
			case "2":
				for _, ms := range g.Spawns() {
					if manager.names(ms.Alias()) {
						ms.SendScriptEvent(2, 0, 0)
					}
				}
			case "3":
				open.set(0)
				for _, ms := range g.Spawns() {
					if manager.names(ms.Alias()) {
						ms.SendScriptEvent(3, 0, 0)
					} else {
						ms.DeleteNPCs()
					}
				}
			}
			base.ScriptEvent(m, e)
		},
		OnNPCDeleted: func(_ *script.Maker, e script.MakerNPC) {
			if open.getOr(0) == 1 {
				e.NPC.ScheduleRespawn(spawn.CalculateRespawnDelay(e.Spawn.Entry()))
			}
		},
		OnNPCCreated: func(_ *script.Maker, e script.MakerNPC) {
			if open.getOr(0) == 0 {
				if !managerOf(e.Group).names(e.Spawn.Alias()) {
					e.NPC.Delete()
				}
			}
		},
	})}
}

// managerAlias is a rift room's manager_npc_name memo.
type managerAlias struct {
	alias    string
	declared bool
}

func managerOf(g spawn.Group) managerAlias {
	alias, ok := g.Maker().AIParams["manager_npc_name"]
	return managerAlias{alias: alias, declared: ok}
}

// names reports whether alias, a spawn's template alias, is the manager's,
// ignoring case. A memo that does not declare the manager panics: the hook
// stops there.
func (a managerAlias) names(alias string) bool {
	if !a.declared {
		panic("maker: memo manager_npc_name is not declared")
	}
	return strings.EqualFold(a.alias, alias)
}
