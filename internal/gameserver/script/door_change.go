package script

import "github.com/fatal10110/acis_golang/internal/gameserver/model/door"

// WatchDoors registers every registered script on the doors its
// ChangedDoors name, in list order, through d: each later opening or
// closing of such a door runs the script's door-change hook, on the
// goroutine that changed the door, after the door's status broadcast and
// before the door it triggers changes. A door id the world has no door for
// registers nothing; a door a reload builds has no registration. Build
// calls it when its Env has doors; a server binding its doors later calls
// it once, then.
func (r *Registry) WatchDoors(d Doors) {
	if r == nil || d == nil {
		return
	}
	for _, e := range r.entries {
		s := e.script
		if s == nil {
			continue
		}
		for _, id := range s.ChangedDoors {
			d.WatchDoor(int(id), door.WatchScript, s, func(obj *door.Object) {
				ev := DoorChange{Door: &Door{obj: obj, doors: d}}
				r.run(s, hookDoorChange, func() { s.Hooks.DoorChange(s, ev) })
			})
		}
	}
}
