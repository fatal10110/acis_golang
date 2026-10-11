package npc

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// AIStringParam returns the AI parameter name, the spawn's value when it
// has one, else the template's; ok is false when neither has it.
func (b *spawnBinding) AIStringParam(name string) (v string, ok bool) {
	if v, ok = b.spawn[name]; ok {
		return v, true
	}
	v, ok = b.templateParams()[name]
	return v, ok
}

// RemoveAllAttackDesire zeroes the NPC's hate of every attacker, drops its
// queued attack desires and stops its movement. The attackers stay listed
// with their damage, and the NPC is not sent back to peace.
func (h *Hostile) RemoveAllAttackDesire() {
	h.brain.CleanAllHate()
	h.move.Stop()
}

// SpawnTerritoryContains reports whether at lies in the territory of the
// spawn that placed the NPC: anywhere for a private, the maker's
// territories less its banned ones for a maker NPC, and a strict sphere of
// the drift range around the spawn point otherwise. An NPC with no spawn
// point has no territory to leave.
func (h *Hostile) SpawnTerritoryContains(at location.Location) bool {
	if h.IsPrivate() {
		return true
	}
	return h.Instance.territoryContains(at)
}

// territoryContains reports whether at lies in the territory of the spawn
// that placed inst: the maker's territories less its banned ones for a
// maker NPC, and a strict sphere of the drift range around the spawn point
// otherwise. An NPC with no spawn point has no territory to leave.
func (inst *Instance) territoryContains(at location.Location) bool {
	if maker := inst.Maker; maker != nil && len(maker.Territories) > 0 {
		if maker.ContainsBanned(at) {
			return false
		}
		return maker.Contains(at)
	}
	if !inst.HasHome {
		return true
	}
	return at.In3DRadius(inst.Home, defaultDriftRange)
}

// InstantTeleportTo moves the NPC to target in one step, staying on the
// world grid: target is grounded unless it lies in water, the observers
// see the jump without its fade, and the NPC's geo-path fail streak
// restarts. Unlike TeleportTo it aborts nothing and forgets no one.
func (h *Hostile) InstantTeleportTo(target location.Location) {
	if h.Live != nil && (h.inWater == nil || !h.inWater(target)) {
		target.Z = int(h.Move().Height(target.X, target.Y, target.Z))
	}
	h.emit(event.Teleported{To: target, Instant: true})
	h.SetXYZ(target.X, target.Y, target.Z)
	h.ResetGeoPathFailCount()
}
