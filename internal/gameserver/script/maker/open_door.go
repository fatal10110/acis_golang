package maker

import "github.com/fatal10110/acis_golang/internal/gameserver/script"

// OpenDoor is the maker whose NPCs live while a door is open: at start it
// registers on the door its DoorName memo names; the door opening spawns
// the group up to the maker's maximum, the door closing deletes it. Its
// NPCs never respawn on their own.
func OpenDoor() script.Maker {
	return script.Maker{MakerHooks: openDoorHooks(&enabledMemo{})}
}

// openDoorHooks are OpenDoor's hooks over its enabled memo: whether the
// door last seen open brought the group in.
func openDoorHooks(enabled *enabledMemo) script.MakerHooks {
	base := defaultHooks()
	return base.With(script.MakerHooks{
		OnStart: func(m *script.Maker, e script.MakerStart) {
			enabled.set(false)
			doorName, ok := e.Group.Maker().AIParams["DoorName"]
			if !ok {
				return
			}
			if d := m.Door(doorName); d != nil {
				d.AddMakerEvent(m, e.Group)
			}
		},
		OnDoorEvent: func(_ *script.Maker, e script.MakerDoor) {
			g := e.Group
			if e.Door.Opened() {
				if enabled.get() {
					return
				}
				enabled.set(true)
				fillToMaximum(g)
				return
			}
			if !enabled.get() {
				return
			}
			enabled.set(false)
			g.DeleteAll()
		},
		OnNPCCreated: func(_ *script.Maker, e script.MakerNPC) {
			if !enabled.get() {
				e.NPC.Delete()
			}
		},
		OnNPCDeleted: func(*script.Maker, script.MakerNPC) {},
	})
}
