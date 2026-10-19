package maker

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// InstantSpawnSerial is the maker of a wave series: its group comes back
// loop_cnt more times, each wave when the last NPC of the previous one
// leaves, after that NPC's respawn delay; the group's last NPC of the
// final wave instead sends script event 1001 to the npcmaker its
// maker_name memo names, with that delay and int2 1. A 1001 with int2 1
// restarts the series. It answers no other script event.
func InstantSpawnSerial() script.Maker {
	loops := &intMemo{key: "i_ai0"}
	base := defaultHooks()
	return script.Maker{MakerHooks: base.With(script.MakerHooks{
		OnStart: func(m *script.Maker, e script.MakerStart) {
			loops.set(memoInt(e.Group, "loop_cnt"))
			base.Start(m, e)
		},
		OnNPCDeleted: func(m *script.Maker, e script.MakerNPC) {
			g := e.Group
			i0 := g.Alive()
			left := loops.get()
			if i0 != 0 {
				return
			}
			switch {
			case left > 0:
				loops.set(left - 1)
				m.ScriptEvent(m, script.MakerEvent{Group: g, Name: "1001", Int1: respawnSeconds(e.Spawn), Int2: 0})
			case left == 0:
				if name, ok := g.Maker().AIParams["maker_name"]; ok {
					g.SendMakerEvent(name, "1001", respawnSeconds(e.Spawn), 1)
				}
			}
		},
		OnScriptEvent: func(m *script.Maker, e script.MakerEvent) {
			if !strings.EqualFold(e.Name, "1001") {
				return
			}
			if e.Int2 == 1 {
				loops.set(memoInt(e.Group, "loop_cnt"))
			}
			base.ScriptEvent(m, e)
		},
	})}
}

// InstantSpawnSerialLast is the last maker of a wave series: its group's
// last NPC leaving sends script event 1001 to the npcmaker its maker_name
// memo names, with that NPC's respawn delay and int2 1, which restarts
// that maker's series.
func InstantSpawnSerialLast() script.Maker {
	base := defaultHooks()
	return script.Maker{MakerHooks: base.With(script.MakerHooks{
		OnNPCDeleted: func(_ *script.Maker, e script.MakerNPC) {
			g := e.Group
			if g.Alive() != 0 {
				return
			}
			if name, ok := g.Maker().AIParams["maker_name"]; ok {
				g.SendMakerEvent(name, "1001", respawnSeconds(e.Spawn), 1)
			}
		},
	})}
}

// respawnSeconds is the spawn's randomized respawn delay in whole seconds,
// the unit a maker script event's delay argument takes.
func respawnSeconds(ms spawn.GroupSpawn) int {
	return int(spawn.CalculateRespawnDelay(ms.Entry()) / time.Second)
}
