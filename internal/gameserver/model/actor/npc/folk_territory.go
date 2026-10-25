package npc

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
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
// grid at target. The NPC lands on the ground at target unless target lies
// in water; for an NPC that can move, a teleport that starts while another
// is under way is dropped (see folkMotion.TeleportTo).
func (f *Folk) TeleportTo(target location.Location) {
	f.TeleportJump(location.Jump{To: target})
}

// TeleportJump is TeleportTo aimed where j lands (move.Land): a scattered
// jump stops at the last point the geodata lets a straight walk from j.To
// toward j.Toward reach, before the landing is grounded. An NPC that
// cannot move lands through FolkRuntime.Landing.
func (f *Folk) TeleportJump(j location.Jump) {
	f.abortAll()
	if f.motion != nil {
		f.motion.teleport(j)
		return
	}
	target := move.Land(f.landing, j, f.inWater)
	f.emit(event.Teleported{To: target})
	f.zones.leave()
	if f.world != nil {
		_ = f.world.Teleport(f, target.X, target.Y, target.Z)
	}
	f.zones.enter()
}

// inWater reports whether at lies in a water zone of the NPC's zone index.
func (f *Folk) inWater(at location.Location) bool {
	if f.zones.ix == nil {
		return false
	}
	_, ok := zone.FindAt[*zone.Water](f.zones.ix, at.X, at.Y, at.Z)
	return ok
}
