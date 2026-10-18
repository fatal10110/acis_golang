package script

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
)

// The door operations of a maker (#3809): the door by name and the
// registration on its openings and closings.

// Door returns the door named name, ignoring case, nil when there is none.
func (m *Maker) Door(name string) *Door {
	d := m.world.Doors
	if d == nil {
		return nil
	}
	obj, ok := d.DoorByName(name)
	if !ok {
		return nil
	}
	return &Door{obj: obj, doors: d}
}

// AddMakerEvent registers the npcmaker g, run by m, on the door: each later
// opening or closing of the door runs m's door event hook, after the
// scripts watching the door and the makers registered before g, on the
// goroutine that changed the door. Registering g again moves it last. A
// door a reload builds keeps no registration.
func (d *Door) AddMakerEvent(m *Maker, g spawn.Group) {
	if m.run == nil {
		panic("script: a maker the registry did not build cannot register on a door")
	}
	run := m.run
	d.doors.WatchDoor(d.obj.DoorID(), door.WatchMaker, g, func(obj *door.Object) {
		run.DoorEvent(g, &Door{obj: obj, doors: d.doors})
	})
}
