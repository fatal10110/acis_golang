package maker

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// The Royal Rush makers of the Four Sepulchers run on the local wall clock:
// an NPC entering the world in minutes 50 to 59 leaves at once, and timer
// 3000, every second from one second after start, clears the group at
// minute 54 second 0 (seconds 1 and 2 again, while any NPC is left).

// RoyalRush is the Royal Rush maker: it spawns nothing at start, and an NPC
// leaving respawns as under the default maker unless its spawn's drawn
// respawn delay is 0 or the minute is 50 to 58.
func RoyalRush() script.Maker {
	return script.Maker{MakerHooks: royalRushHooks()}
}

// royalRushHooks are RoyalRush's hooks, the parent of RoyalReqNext and
// RoyalSpawnTreasureBox. Timer 3000 runs the maker's own script event 1000.
func royalRushHooks() script.MakerHooks {
	base := defaultHooks()
	return base.With(script.MakerHooks{
		OnStart: func(m *script.Maker, e script.MakerStart) {
			g := e.Group
			g.Every(time.Second, func() { m.Timer(m, script.MakerTimer{Group: g, Name: "3000"}) })
		},
		OnNPCCreated: func(m *script.Maker, e script.MakerNPC) {
			if currentMinute := m.Now().Minute(); currentMinute > 49 && currentMinute < 60 {
				e.NPC.Delete()
			}
		},
		OnNPCDeleted: func(m *script.Maker, e script.MakerNPC) {
			currentMinute := m.Now().Minute()
			if spawn.CalculateRespawnDelay(e.Spawn.Entry()) != 0 && !(currentMinute > 49 && currentMinute < 59) {
				base.NPCDeleted(m, e)
			}
		},
		OnTimer: func(m *script.Maker, e script.MakerTimer) {
			if !strings.EqualFold(e.Name, "3000") {
				return
			}
			now := m.Now()
			if now.Minute() != 54 {
				return
			}
			switch now.Second() {
			case 0:
				m.ScriptEvent(m, script.MakerEvent{Group: e.Group, Name: "1000"})
			case 1, 2:
				if e.Group.Alive() > 0 {
					m.ScriptEvent(m, script.MakerEvent{Group: e.Group, Name: "1000"})
				}
			}
		},
	})
}

// RoyalReqNext is the Royal Rush maker whose NPCs never respawn on their
// own; it calls the npcmaker its next_maker_name memo names. A boss maker
// (BossMaker memo 1) runs 1002 there when one of the four sepulcher bosses
// leaves. Any other runs 1001 there in minutes 0 to 50 when an NPC leaving
// brings its count down to its req_count memo.
func RoyalReqNext() script.Maker {
	base := royalRushHooks()
	return script.Maker{MakerHooks: base.With(script.MakerHooks{
		OnNPCDeleted: func(m *script.Maker, e script.MakerNPC) {
			g := e.Group
			if memoIntOr(g, "BossMaker", -1) == 1 {
				switch e.Spawn.Entry().NPCID {
				case 25339, 25342, 25346, 25349:
					sendNextMaker(g, "1002")
				}
				return
			}
			if g.Alive() == memoIntOr(g, "req_count", -1) {
				if currentMinute := m.Now().Minute(); currentMinute >= 0 && currentMinute <= 50 {
					sendNextMaker(g, "1001")
				}
			}
		},
	})}
}

// sendNextMaker runs the maker script event name on the npcmaker g's
// next_maker_name memo names; nothing when the memo names none.
func sendNextMaker(g spawn.Group, name string) {
	if next, ok := g.Maker().AIParams["next_maker_name"]; ok {
		g.SendMakerEvent(next, name, 0, 0)
	}
}

// RoyalSpawnTreasureBox is the Royal Rush maker of a sepulcher's treasure
// chests. Script event 1002, the only one it answers, brings Rnd(10) more
// chests of its first spawn than the minute allows (10 from minute 48, up to
// 157 before minute 12), when they all fit the maximum.
func RoyalSpawnTreasureBox() script.Maker {
	base := royalRushHooks()
	return script.Maker{MakerHooks: base.With(script.MakerHooks{
		OnScriptEvent: func(m *script.Maker, e script.MakerEvent) {
			if !strings.EqualFold(e.Name, "1002") {
				return
			}
			g := e.Group
			def0 := firstSpawn(g)
			if def0 == nil {
				return
			}
			i1 := m.Now().Minute()
			i2 := m.Rnd(10)
			switch {
			case i1 >= 48:
				i2 += 10
			case i1 >= 46:
				i2 += 15
			case i1 >= 44:
				i2 += 20
			case i1 >= 42:
				i2 += 26
			case i1 >= 40:
				i2 += 32
			case i1 >= 38:
				i2 += 39
			case i1 >= 36:
				i2 += 45
			case i1 >= 34:
				i2 += 52
			case i1 >= 32:
				i2 += 60
			case i1 >= 30:
				i2 += 68
			case i1 >= 28:
				i2 += 76
			case i1 >= 26:
				i2 += 85
			case i1 >= 24:
				i2 += 94
			case i1 >= 22:
				i2 += 103
			case i1 >= 20:
				i2 += 113
			case i1 >= 16:
				i2 += 123
			case i1 >= 14:
				i2 += 134
			case i1 >= 12:
				i2 += 145
			default:
				i2 += 157
			}
			if g.Alive()+i2 <= g.Maker().MaximumNPCs {
				for range i2 {
					if def0.Decayed() > 0 {
						def0.Respawn()
					} else {
						def0.Spawn()
					}
				}
			}
		},
	})}
}
