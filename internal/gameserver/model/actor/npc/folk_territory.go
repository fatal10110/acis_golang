package npc

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// InTerritory reports whether the NPC stands in the territory of the spawn
// that placed it: its maker's territories less the banned ones, else a
// strict sphere of the drift range around its spawn point. An NPC with no
// spawn point is always in it. A private is in its master's territory, as
// a hostile private is.
func (f *Folk) InTerritory() bool {
	if in, decided := f.privateInTerritory(); decided {
		return in
	}
	return f.SpawnTerritoryContains(f.location())
}

// TeleportTo jumps the NPC to target. It stops its walk, swing and cast,
// then observers see the jump and the NPC leaves and re-enters the world
// grid at target. An NPC that can move lands on the ground at target unless
// target lies in water; a teleport that starts while another is under way
// is dropped (see folkMotion.TeleportTo). One that cannot move keeps
// target's height.
func (f *Folk) TeleportTo(target location.Location) {
	f.TeleportJump(location.Jump{To: target})
}

// TeleportJump is TeleportTo aimed where j lands: an NPC that can move
// stops, on a scattered jump, at the last point the geodata lets a
// straight walk from j.To toward j.Toward reach, before the landing is
// grounded; one that cannot move lands on j.To.
func (f *Folk) TeleportJump(j location.Jump) {
	f.abortAll()
	if f.motion != nil {
		f.motion.teleport(j)
		return
	}
	target := j.To
	f.emit(event.Teleported{To: target})
	f.zones.leave()
	if f.world != nil {
		_ = f.world.Teleport(f, target.X, target.Y, target.Z)
	}
	f.zones.enter()
}
