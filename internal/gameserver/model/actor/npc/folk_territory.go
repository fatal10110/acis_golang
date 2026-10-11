package npc

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// InTerritory reports whether the NPC stands in the territory of the spawn
// that placed it: its maker's territories less the banned ones, else a
// strict sphere of the drift range around its spawn point. An NPC with no
// spawn point is always in it.
func (f *Folk) InTerritory() bool { return f.SpawnTerritoryContains(f.location()) }

// TeleportTo jumps the NPC to target. It stops its walk, swing and cast,
// then observers see the jump and the NPC leaves and re-enters the world
// grid at target. An NPC that can move lands on the ground at target unless
// target lies in water; a teleport that starts while another is under way
// is dropped (see folkMotion.TeleportTo). One that cannot move keeps
// target's height.
func (f *Folk) TeleportTo(target location.Location) {
	f.abortAll()
	if f.motion != nil {
		f.motion.TeleportTo(target)
		return
	}
	f.emit(event.Teleported{To: target})
	f.zones.leave(f.location())
	if f.world != nil {
		_ = f.world.Teleport(f, target.X, target.Y, target.Z)
	}
	f.zones.enter()
}
