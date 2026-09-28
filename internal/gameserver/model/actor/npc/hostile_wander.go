package npc

import (
	"github.com/fatal10110/acis_golang/internal/commons/rnd"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
)

// minWanderOffset is Npc.moveFromSpawnPointUsingRandomOffset's "offset
// isn't noticeable" cutoff: offsets below this do not start a walk.
const minWanderOffset = 10

// randomWalkLoopLimit is how many offset samples a maker wander tries
// before aiming at the current triangle's center.
const randomWalkLoopLimit = 3

// ShouldIdleWander reports whether an empty desire queue should become a
// wander desire. Hold-position kinds stay put. MovingAttack is not a
// wander gate (Warrior.java:331-334, Wizard.java:28-31); only
// MonsterBehavior.onNoDesire reads it. Script-accurate eligibility: #2148.
func (h *Hostile) ShouldIdleWander() bool {
	switch hostileKind(h.Instance) {
	case "Guard", "SiegeGuard", "Chest", "HalishaChest":
		return false
	default:
		return true
	}
}

// RealMoveSpeed is the stance-aware move speed used as the wander offset
// basis (walk after thinkWander switches stance).
func (h *Hostile) RealMoveSpeed() float64 {
	return float64(h.moveSpeed())
}

// MoveFromSpawnUsingRandomOffset walks toward a geo-validated wander point.
// Maker NPCs sample from the current position inside the maker territory.
// Privates (a live master, no maker) offset from the NPC's current XY.
// Other nil-maker spawns offset from spawn home. No spawn point or a
// sub-noticeable offset is a no-op, as is a movement-disabled NPC: it keeps
// its wander intention but starts no walk.
func (h *Hostile) MoveFromSpawnUsingRandomOffset(offset int) {
	if h.Instance == nil || !h.Instance.HasHome || offset < minWanderOffset || h.MovementDisabled() {
		return
	}
	from := h.location()
	dest, ok := h.randomWalkLocation(from, offset)
	if !ok || dest == from {
		return
	}
	_, _ = h.move.MoveToLocation(dest)
}

func (h *Hostile) randomWalkLocation(from location.Location, offset int) (location.Location, bool) {
	maker := h.Instance.Maker
	if maker == nil || len(maker.Territories) == 0 {
		return h.homeOffsetWalk(from, offset), true
	}
	return h.makerWalkLocation(maker, from, offset)
}

func (h *Hostile) homeOffsetWalk(from location.Location, offset int) location.Location {
	dest := h.Instance.Home
	if h.Master() != nil {
		dest = from
	}
	dest.X += rnd.GetRange(-offset, offset)
	dest.Y += rnd.GetRange(-offset, offset)
	return h.ValidLocation(from.X, from.Y, from.Z, dest.X, dest.Y, dest.Z)
}

func (h *Hostile) makerWalkLocation(maker *spawn.Maker, from location.Location, offset int) (location.Location, bool) {
	shape, ok := maker.ContainingTriangle(from.X, from.Y)
	if !ok {
		// Defensive: idle wander only calls this after InTerritory is true,
		// which already requires a containing footprint, so this miss is
		// not reached from that caller. Keep the territory-wide sample for
		// a current-triangle lookup miss on the public walk helper.
		return h.makerRandomLocation(maker)
	}
	for range randomWalkLoopLimit {
		loc := from.AddRandomOffsetBetween(offset/rnd.GetRange(2, 4), offset)
		if !maker.Contains(loc) || maker.ContainsBanned(loc) {
			continue
		}
		return h.ValidLocation(from.X, from.Y, from.Z, loc.X, loc.Y, loc.Z), true
	}
	center := shape.Center()
	return h.ValidLocation(from.X, from.Y, from.Z, center.X, center.Y, from.Z), true
}

// makerRandomLocation draws an out-of-territory wander destination from
// the whole maker territory. Unlike spawn placement it does not avoid the
// banned territory.
func (h *Hostile) makerRandomLocation(maker *spawn.Maker) (location.Location, bool) {
	return maker.RandomLocation(h.Move(), false)
}
