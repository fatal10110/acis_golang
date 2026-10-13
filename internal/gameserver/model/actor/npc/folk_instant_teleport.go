package npc

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// InstantTeleportTo moves the civilian NPC in one step to where j lands, as
// a hostile NPC's InstantTeleportTo does: an NPC that can move lands on the
// ground unless the point lies in water, one that cannot keeps j's point;
// observers see the jump without its fade; the NPC stays in its region
// until it next moves, so what knows it, what it knows and the zones it is
// in are unchanged; and its failed-pathfinding streak clears. It aborts
// nothing and forgets no one.
func (f *Folk) InstantTeleportTo(j location.Jump) {
	m := f.motion
	if m == nil {
		target := j.To
		f.emit(event.Teleported{To: target, Instant: true})
		if f.world != nil {
			f.world.SetPosition(f, target.X, target.Y, target.Z)
		}
		return
	}
	target := move.Land(m.move, j, m.cfg.InWater)
	m.emit(event.Teleported{To: target, Instant: true})
	m.move.SetPosition(target)
	if m.cfg.World != nil {
		m.cfg.World.SetPosition(f, target.X, target.Y, target.Z)
	}
	m.ResetGeoPathFailCount()
}
