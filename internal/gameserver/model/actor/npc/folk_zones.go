package npc

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
)

// EnterZones enters the NPC into the zones at its position. Call it once
// the NPC is spawned into the world.
func (f *Folk) EnterZones() { f.zones.enter() }

// InsideZone reports whether the NPC's zones hold flag.
func (f *Folk) InsideZone(flag zone.Flag) bool { return f.zones.has(flag) }

func (f *Folk) zoneTeleporting() bool {
	return f.motion != nil && f.motion.teleporting.Load()
}

// swimStateChanged switches the NPC's movement, if it has any, into or out
// of swimming and shows its observers its new state: the stationary view
// for an NPC that cannot move at its current speed, the full one otherwise.
func (f *Folk) swimStateChanged(swimming bool) {
	if f.motion != nil {
		f.motion.move.SetSwimming(swimming)
	}
	f.emit(event.NPCInfoChanged{ServerObject: f.MoveSpeed() == 0})
}
