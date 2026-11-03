package maker

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// FrintezzaEvilate is the maker of the Evil Spirits of Frintezza's tomb:
// the default maker whose script event 1001 fills every spawn to its own
// total, whatever the maker's maximum. 1000 deletes every NPC of the group;
// 1001, unless the spawn condition holds, arms the respawn of each NPC of a
// spawn out of the world and brings in the rest the spawn is short of,
// each after int1 seconds.
func FrintezzaEvilate() script.Maker {
	base := defaultHooks()
	return script.Maker{MakerHooks: base.With(script.MakerHooks{
		OnScriptEvent: func(_ *script.Maker, e script.MakerEvent) {
			g := e.Group
			switch {
			case strings.EqualFold(e.Name, "1000"):
				g.DeleteAll()
			case strings.EqualFold(e.Name, "1001"):
				if g.Held() {
					return
				}
				delay := time.Duration(e.Int1) * time.Second
				for _, ms := range g.Spawns() {
					toSpawn := ms.Total() - ms.Spawned()
					for _, npc := range ms.NPCs() {
						if npc.Decayed() {
							npc.ScheduleRespawn(delay)
							toSpawn--
						}
					}
					for range max(toSpawn, 0) {
						g.After(delay, func() { respawnOrSpawn(ms) })
					}
				}
			}
		},
	})}
}
