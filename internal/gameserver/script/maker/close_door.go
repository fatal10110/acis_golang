package maker

import "github.com/fatal10110/acis_golang/internal/gameserver/script"

// CloseDoor is the maker whose NPCs live while a door is closed: at start
// it registers on the door its DoorName memo names; the door closing
// spawns the group up to the maker's maximum, the door opening deletes it.
// Its NPCs never respawn on their own.
func CloseDoor() script.Maker {
	// memo holds the maker memo's "enabled", which start writes: whether
	// the door last seen closed brought the group in.
	var memo script.MapCell[string, bool]
	enabled := func() bool {
		v, ok := memo.Get("enabled")
		if !ok {
			panic("maker: memo enabled read before the maker started")
		}
		return v
	}
	base := defaultHooks()
	return script.Maker{MakerHooks: base.With(script.MakerHooks{
		OnStart: func(m *script.Maker, e script.MakerStart) {
			memo.Set("enabled", false)
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
				if enabled() {
					return
				}
				memo.Set("enabled", true)
				for _, ms := range g.Spawns() {
					for range ms.Total() - ms.Spawned() {
						if g.Maker().MaximumNPCs-g.Alive() > 0 {
							ms.Spawn()
						}
					}
				}
				return
			}
			if !enabled() {
				return
			}
			memo.Set("enabled", false)
			g.DeleteAll()
		},
		OnNPCCreated: func(_ *script.Maker, e script.MakerNPC) {
			if !enabled() {
				e.NPC.Delete()
			}
		},
		OnNPCDeleted: func(*script.Maker, script.MakerNPC) {},
	})}
}
