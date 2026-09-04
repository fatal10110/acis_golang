package npc

import (
	"github.com/fatal10110/acis_golang/internal/commons/rnd"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/geometry"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
)

// minWanderOffset is Npc.moveFromSpawnPointUsingRandomOffset's "offset
// isn't noticeable" cutoff: offsets below this do not start a walk.
const minWanderOffset = 10

// randomWalkLoopLimit is how many offset samples a maker wander tries
// before aiming at the current triangle's center.
const randomWalkLoopLimit = 3

// territoryWalkAttempts is how many area-weighted samples an out-of-territory
// maker wander takes before accepting the last estimated point.
const territoryWalkAttempts = 10

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
// sub-noticeable offset is a no-op.
func (h *Hostile) MoveFromSpawnUsingRandomOffset(offset int) {
	if h.Instance == nil || !h.Instance.HasHome || offset < minWanderOffset {
		return
	}
	from := h.location()
	dest, ok := h.randomWalkLocation(from, offset)
	if !ok || dest == from {
		return
	}
	mover, ok := h.move.(interface {
		MoveToLocation(location.Location) (bool, error)
	})
	if !ok {
		return
	}
	_, _ = mover.MoveToLocation(dest)
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

func (h *Hostile) makerRandomLocation(maker *spawn.Maker) (location.Location, bool) {
	triangles := maker.Triangles()
	if len(triangles) == 0 {
		return location.Location{}, false
	}
	minZ, maxZ, ok := maker.MergedZRange()
	if !ok {
		return location.Location{}, false
	}
	avgZ := (minZ + maxZ) / 2
	var last location.Location
	have := false
	for failed := 0; failed < territoryWalkAttempts; {
		tri, ok := pickWeightedTriangle(triangles)
		if !ok {
			break
		}
		pt := randomPointInTriangle(tri)
		z := int(h.Move().Height(pt.X, pt.Y, avgZ))
		last = location.Location{X: pt.X, Y: pt.Y, Z: z}
		have = true
		if z < minZ || z > maxZ || !h.Move().Walkable(pt.X, pt.Y, z) {
			failed++
			continue
		}
		return last, true
	}
	return last, have
}

func pickWeightedTriangle(triangles []geometry.Triangle) (geometry.Triangle, bool) {
	if len(triangles) == 0 {
		return geometry.Triangle{}, false
	}
	if len(triangles) == 1 {
		return triangles[0], true
	}
	var total int64
	for _, tri := range triangles {
		total += tri.Size()
	}
	if total <= 0 {
		return triangles[rnd.Get(len(triangles))], true
	}
	roll := int64(rnd.GetFloat(float64(total)))
	for _, tri := range triangles {
		roll -= tri.Size()
		if roll < 0 {
			return tri, true
		}
	}
	return triangles[len(triangles)-1], true
}

func randomPointInTriangle(tri geometry.Triangle) geometry.Point {
	v := tri.Vertices()
	ba := rnd.GetFloat(1)
	ca := rnd.GetFloat(1)
	if ba+ca > 1 {
		ba = 1 - ba
		ca = 1 - ca
	}
	bax := float64(v[1].X - v[0].X)
	bay := float64(v[1].Y - v[0].Y)
	cax := float64(v[2].X - v[0].X)
	cay := float64(v[2].Y - v[0].Y)
	return geometry.Point{
		X: v[0].X + int(ba*bax+ca*cax),
		Y: v[0].Y + int(ba*bay+ca*cay),
	}
}
