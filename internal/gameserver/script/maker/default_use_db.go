package maker

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// DefaultUseDB is the maker of database-tracked NPCs: at start every spawn
// short of its total runs the npc DB info hook, which places one NPC. An
// NPC leaving the world clears its spawn's loaded mark; the group's last
// one sends script event 1001 to the npcmaker its maker_name memo names,
// and while the group is under its maximum the spawn brings an NPC back
// after its respawn delay.
func DefaultUseDB() script.Maker {
	base := defaultHooks()
	return script.Maker{MakerHooks: base.With(script.MakerHooks{
		OnStart: func(_ *script.Maker, e script.MakerStart) {
			for _, ms := range e.Group.Spawns() {
				if ms.Total() != ms.Spawned() {
					ms.LoadDBInfo()
				}
			}
		},
		OnNPCDBInfo: func(_ *script.Maker, e script.MakerSpawn) {
			if e.Spawn.Total()-e.Spawn.Spawned() > 0 {
				e.Spawn.Spawn()
			}
		},
		OnNPCDeleted: func(_ *script.Maker, e script.MakerNPC) {
			g, ms := e.Group, e.Spawn
			ms.ClearDBLoaded()
			if g.Alive() == 0 {
				if name, ok := g.Maker().AIParams["maker_name"]; ok {
					g.SendMakerEvent(name, "1001", 0, 0)
				}
			}
			if g.Maker().MaximumNPCs-g.Alive() > 0 {
				g.After(spawn.CalculateRespawnDelay(ms.Entry()), func() {
					if ms.Decayed() > 0 {
						ms.Respawn()
					} else {
						ms.Spawn()
					}
				})
			}
		},
	})}
}
