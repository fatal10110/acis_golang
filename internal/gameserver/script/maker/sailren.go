package maker

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// Sailren is the maker of Sailren: DefaultUseDB's maker. Maker script
// event 11042 sends its first spawn's NPCs 11042 with 1 and 0; every
// script event then runs the default maker's.
//
// Its npc DB info hook, for a row that is not saved dead with its respawn
// still ahead, places Sailren while the group is under its maximum; a row
// whose saved X is not the lair's -113091 first runs 11047 on
// rune16_npc2017_01m1 and rune16_npc2017_13m1. A row saved dead, or a
// spawn that keeps no row, has the spawn place him as it would, the
// maximum unchecked. Sailren leaving runs 11045 on rune16_npc2017_01m1
// and, while the group is under its maximum, brings him back after the
// spawn's respawn delay; his row keeps its loaded mark.
func Sailren() script.Maker {
	base := DefaultUseDB().MakerHooks
	return script.Maker{MakerHooks: base.With(script.MakerHooks{
		OnScriptEvent: func(m *script.Maker, e script.MakerEvent) {
			if strings.EqualFold(e.Name, "11042") {
				if def0 := firstSpawn(e.Group); def0 != nil {
					def0.SendScriptEvent(11042, 1, 0)
				}
			}
			base.ScriptEvent(m, e)
		},
		OnNPCDBInfo: func(_ *script.Maker, e script.MakerSpawn) {
			g, ms := e.Group, e.Spawn
			if ms.Persisted() && !ms.CheckDead() {
				if g.Maker().MaximumNPCs > g.Alive() {
					if ms.SavedX() != -113091 {
						g.SendMakerEvent("rune16_npc2017_01m1", "11047", 0, 0)
						g.SendMakerEvent("rune16_npc2017_13m1", "11047", 0, 0)
					}
					ms.Spawn()
				}
			} else {
				ms.Spawn()
			}
		},
		OnNPCDeleted: func(_ *script.Maker, e script.MakerNPC) {
			e.Group.SendMakerEvent("rune16_npc2017_01m1", "11045", 0, 0)

			if e.Group.Maker().MaximumNPCs > e.Group.Alive() {
				e.NPC.ScheduleRespawn(spawn.CalculateRespawnDelay(e.Spawn.Entry()))
			}
		},
	})}
}
