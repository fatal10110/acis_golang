package script

import (
	"fmt"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
)

// Zone is a script's handle on one zone of the world.
type Zone struct {
	k zone.Kind
}

// Zone returns the zone id, nil when there is none.
func (s *Script) Zone(id int) *Zone {
	if s.env.Zones == nil {
		return nil
	}
	k, ok := s.env.Zones.ByID(id)
	if !ok {
		return nil
	}
	return &Zone{k: k}
}

// BossZoneAt returns the first grand boss zone, among the zones of at's
// world region, whose volume holds at; nil when none does.
func (s *Script) BossZoneAt(at Place) *Zone {
	if s.env.Zones == nil {
		return nil
	}
	l := at.loc()
	for _, k := range s.env.Zones.At(l.X, l.Y) {
		if _, ok := k.(*zone.Boss); ok && k.Core().ContainsPoint(l.X, l.Y, l.Z) {
			return &Zone{k: k}
		}
	}
	return nil
}

// Contains reports whether at lies inside the zone's volume.
func (z *Zone) Contains(at Place) bool {
	l := at.loc()
	return z.k.Core().ContainsPoint(l.X, l.Y, l.Z)
}

// AllowPlayerEntry lets p into the grand boss zone for the next seconds:
// p is added to the players the zone admits, and may enter until that
// time has passed. A zone that is not a grand boss zone panics.
func (z *Zone) AllowPlayerEntry(p *Player, seconds int) {
	boss, ok := z.k.(*zone.Boss)
	if !ok {
		panic(fmt.Sprintf("script: entry to zone %d, which is no grand boss zone", z.k.Core().ID()))
	}
	boss.AllowEntry(p.ObjectID(), time.Duration(seconds)*time.Second)
}
