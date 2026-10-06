package network

import "github.com/fatal10110/acis_golang/internal/gameserver/model/zone"

// wireScriptZones makes a player's entry into a zone a script reacts to
// reach that script. The zone notes the entry on the player while its
// revalidation holds the player's zone lock; the revalidation hands it to
// the script once that lock is released. Only players are noted: the one
// script that reacts to a zone entry reacts to players alone.
func (l *GameClientLink) wireScriptZones() {
	if l.scripts == nil || l.zones == nil {
		return
	}
	for _, id := range l.scripts.ZoneEnterIDs() {
		k, ok := l.zones.ByID(int(id))
		if !ok {
			continue
		}
		k.Core().OnEnter(func(a zone.Actor) {
			if p, ok := a.(*liveZoneActor); ok {
				p.noteScriptZone(id)
			}
		})
	}
}
