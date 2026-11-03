package maker

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// Velociraptor is the maker of the velociraptors of Sailren's lair, the
// first of the lair's three dinosaur makers, keeping a flag (memo i_ai0)
// that start sets to 0 and spawns nothing.
func Velociraptor() script.Maker {
	flag := &intMemo{key: "i_ai0"}
	return script.Maker{MakerHooks: velociraptorHooks(flag)}
}

// velociraptorHooks are Velociraptor's hooks over its flag memo (i_ai0).
//
// Maker script event 1001 brings the group up to its maximum: each NPC
// comes in after int1 seconds, an NPC out of the world back before a new
// one, and each arms timer 1001 five seconds out. 11052 raises the flag
// and 11053 lowers it; every script event but 1001 then runs the default
// maker's.
//
// Timer 1001 sends every spawn's NPCs 11049 and arms timer 78001 five
// hours out. Timer 78001, while the flag is down, runs 11052 and then 1000
// on rune20_mb2017_01m1, 02m1 and 03m1, and 11050 on rune16_npc2017_01m1.
//
// The group's last NPC leaving, while the flag is down, runs 1001 on
// rune20_mb2017_02m1. Its NPCs never respawn on their own.
func velociraptorHooks(flag *intMemo) script.MakerHooks {
	base := defaultHooks()
	return base.With(script.MakerHooks{
		OnStart: func(*script.Maker, script.MakerStart) {
			flag.set(0)
		},
		OnScriptEvent: func(m *script.Maker, e script.MakerEvent) {
			g := e.Group
			switch {
			case strings.EqualFold(e.Name, "1001"):
				totalUnspawned := g.Maker().MaximumNPCs - g.Alive()
				if totalUnspawned <= 0 {
					return
				}
				delay := time.Duration(e.Int1) * time.Second
				for _, ms := range g.Spawns() {
					toSpawn := min(totalUnspawned, ms.Total()-ms.Spawned())
					for range max(toSpawn, 0) {
						g.After(delay, func() { respawnOrSpawn(ms) })
						g.After(5000*time.Millisecond, func() { m.Timer(m, script.MakerTimer{Group: g, Name: "1001"}) })
						totalUnspawned--
					}
				}
				return
			case strings.EqualFold(e.Name, "11052"):
				flag.set(1)
			case strings.EqualFold(e.Name, "11053"):
				flag.set(0)
			}
			base.ScriptEvent(m, e)
		},
		OnTimer: func(m *script.Maker, e script.MakerTimer) {
			g := e.Group
			switch {
			case strings.EqualFold(e.Name, "1001"):
				for _, ms := range g.Spawns() {
					ms.SendScriptEvent(11049, 0, 0)
				}
				g.After(18000*time.Second, func() { m.Timer(m, script.MakerTimer{Group: g, Name: "78001"}) })
			case strings.EqualFold(e.Name, "78001"):
				if flag.get() != 0 {
					return
				}
				for _, name := range []string{"rune20_mb2017_01m1", "rune20_mb2017_02m1", "rune20_mb2017_03m1"} {
					g.SendMakerEvent(name, "11052", 0, 0)
					g.SendMakerEvent(name, "1000", 0, 0)
				}
				g.SendMakerEvent("rune16_npc2017_01m1", "11050", 0, 0)
			}
		},
		OnNPCDeleted: func(_ *script.Maker, e script.MakerNPC) {
			if e.Group.Alive() == 0 && flag.get() == 0 {
				e.Group.SendMakerEvent("rune20_mb2017_02m1", "1001", 0, 0)
			}
		},
	})
}

// respawnOrSpawn brings in one NPC of ms: one out of the world back if
// the spawn keeps any, else a new one.
func respawnOrSpawn(ms spawn.GroupSpawn) {
	if ms.Decayed() > 0 {
		ms.Respawn()
	} else {
		ms.Spawn()
	}
}
