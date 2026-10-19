package maker

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// CloseDoor is the maker whose NPCs live while a door is closed: at start
// it registers on the door its DoorName memo names; the door closing
// spawns the group up to the maker's maximum, the door opening deletes it.
// Its NPCs never respawn on their own.
func CloseDoor() script.Maker {
	return script.Maker{MakerHooks: closeDoorHooks(&enabledMemo{})}
}

// closeDoorHooks are CloseDoor's hooks over its enabled memo.
func closeDoorHooks(enabled *enabledMemo) script.MakerHooks {
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
			if !e.Door.Opened() {
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

// enabledMemo is the maker memo's "enabled", which start writes: whether
// the door last seen closed brought the group in.
type enabledMemo struct {
	cell script.MapCell[string, bool]
}

func (c *enabledMemo) get() bool {
	v, ok := c.cell.Get("enabled")
	if !ok {
		panic("maker: memo enabled read before the maker started")
	}
	return v
}

func (c *enabledMemo) set(v bool) { c.cell.Set("enabled", v) }

// fillToMaximum fills every spawn of g up to its total, one NPC at a time
// while the group is under its maximum.
func fillToMaximum(g spawn.Group) {
	for _, ms := range g.Spawns() {
		for range ms.Total() - ms.Spawned() {
			if g.Maker().MaximumNPCs-g.Alive() > 0 {
				ms.Spawn()
			}
		}
	}
}
