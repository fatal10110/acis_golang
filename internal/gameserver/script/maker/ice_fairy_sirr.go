package maker

import (
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// IceFairySirr is the maker of Ice Fairy Sirra: CloseDoor's maker, keeping
// a count (memo i_ai0) that start sets to 0 before CloseDoor's start.
//
// Its door closing spawns the group up to the maker's maximum, as
// CloseDoor's does, then runs 11037 on schuttgart13_npc2314_1m1; its door
// opening deletes the group. Maker script event 10005 adds one to the
// count and sends the first spawn's NPCs 10001 with it; 11040 sends them
// 11040 with int1. It answers no default maker script event. An NPC
// leaving sets the count back to 0 and arms no respawn.
func IceFairySirr() script.Maker {
	count := &intMemo{key: "i_ai0"}
	enabled := &enabledMemo{}
	base := closeDoorHooks(enabled)
	return script.Maker{MakerHooks: base.With(script.MakerHooks{
		OnStart: func(m *script.Maker, e script.MakerStart) {
			count.set(0)
			base.Start(m, e)
		},
		OnScriptEvent: func(_ *script.Maker, e script.MakerEvent) {
			def0 := firstSpawn(e.Group)
			if def0 == nil {
				return
			}
			switch {
			case strings.EqualFold(e.Name, "10005"):
				count.set(count.get() + 1)
				def0.SendScriptEvent(10001, int32(count.get()), 0)
			case strings.EqualFold(e.Name, "11040"):
				def0.SendScriptEvent(11040, int32(e.Int1), 0)
			}
		},
		OnDoorEvent: func(_ *script.Maker, e script.MakerDoor) {
			g := e.Group
			if !e.Door.Opened() {
				if enabled.get() {
					return
				}
				enabled.set(true)
				fillToMaximum(g)
				g.SendMakerEvent("schuttgart13_npc2314_1m1", "11037", 0, 0)
				return
			}
			if !enabled.get() {
				return
			}
			enabled.set(false)
			g.DeleteAll()
		},
		OnNPCDeleted: func(*script.Maker, script.MakerNPC) {
			count.set(0)
		},
	})}
}
