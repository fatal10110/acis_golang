package maker

import (
	"strings"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// OnDayNightSpawn is the maker whose NPCs live only at night (IsNight memo
// 1) or only by day (0). At start the group spawns when the time of day
// fits; a check one second later and then every minute brings it in with
// script event 1001 once the time of day fits, and deletes it with 1000
// once it no longer does. An NPC appearing while the group is out and the
// time of day does not fit is deleted; an NPC leaving respawns only while
// the time of day fits and the group is under its maximum.
func OnDayNightSpawn() script.Maker {
	// spawned is the memo's i_ai0: whether the group is in for the
	// current time of day.
	spawned := &intMemo{key: "i_ai0"}
	base := defaultHooks()
	return script.Maker{MakerHooks: base.With(script.MakerHooks{
		OnStart: func(m *script.Maker, e script.MakerStart) {
			isNight := memoInt(e.Group, "IsNight")
			if m.IsNight() {
				if isNight == 1 {
					spawned.set(1)
					base.Start(m, e)
				} else {
					spawned.set(0)
				}
			} else if isNight == 0 {
				spawned.set(1)
				base.Start(m, e)
			} else {
				spawned.set(0)
			}
			g := e.Group
			check := func() { m.Timer(m, script.MakerTimer{Group: g, Name: "3000"}) }
			g.After(time.Second, func() {
				check()
				g.Every(time.Minute, check)
			})
		},
		OnNPCCreated: func(m *script.Maker, e script.MakerNPC) {
			if spawned.get() != 0 {
				return
			}
			isNight := memoInt(e.Group, "IsNight")
			if m.IsNight() {
				if isNight == 0 {
					e.NPC.Delete()
				}
			} else if isNight == 1 {
				e.NPC.Delete()
			}
		},
		OnNPCDeleted: func(m *script.Maker, e script.MakerNPC) {
			g := e.Group
			isNight := memoInt(g, "IsNight")
			if m.IsNight() {
				if isNight == 1 && g.Maker().MaximumNPCs-g.Alive() > 0 {
					e.NPC.ScheduleRespawn(spawn.CalculateRespawnDelay(e.Spawn.Entry()))
				}
			} else if isNight == 0 && g.Maker().MaximumNPCs-g.Alive() > 0 {
				e.NPC.ScheduleRespawn(spawn.CalculateRespawnDelay(e.Spawn.Entry()))
			}
		},
		OnTimer: func(m *script.Maker, e script.MakerTimer) {
			if !strings.EqualFold(e.Name, "3000") {
				return
			}
			in := spawned.get()
			isNight := memoInt(e.Group, "IsNight")
			bringIn := func() {
				m.ScriptEvent(m, script.MakerEvent{Group: e.Group, Name: "1001"})
				spawned.set(1)
			}
			takeOut := func() {
				m.ScriptEvent(m, script.MakerEvent{Group: e.Group, Name: "1000"})
				spawned.set(0)
			}
			if m.IsNight() {
				if in == 0 {
					if isNight == 1 {
						bringIn()
					}
				} else if isNight == 0 {
					takeOut()
				}
			} else if in == 0 {
				if isNight == 0 {
					bringIn()
				}
			} else if isNight == 1 {
				takeOut()
			}
		},
	})}
}
