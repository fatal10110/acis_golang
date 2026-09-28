package spawn

import (
	"github.com/fatal10110/acis_golang/internal/commons/rnd"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/geometry"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// Terrain is the geodata a territory-random draw resolves against: the
// ground height nearest a seed Z, and whether a point is open ground.
type Terrain interface {
	Height(x, y, z int) int16
	Walkable(x, y, z int) bool
}

// randomLocationAttempts is how many out-of-range-Z or unwalkable draws a
// territory-random location tolerates before settling for the last draw.
const randomLocationAttempts = 10

// randomLocationDrawLimit caps every draw, including those rejected for
// landing in the banned territory, which do not spend the attempt budget.
// It only bites when nearly the whole territory is banned, where an
// uncapped loop would never end. A banned draw is never the fallback: the
// capped call returns the last draw that failed only on Z or walkability,
// or nothing when every draw was banned.
const randomLocationDrawLimit = 10000

// RandomLocation draws a point in this maker's merged territory: a triangle
// is picked with probability proportional to its size across every member
// shape, a uniform point is taken inside it, and Z is the geodata height
// nearest the merged Z midpoint. A draw whose Z leaves the merged range or
// whose point is not walkable counts against the attempt budget; with
// excludeBanned, a draw inside the merged banned territory is redrawn
// without counting. Once the budget runs out the last draw is returned
// as-is. ok is false when the territory has no drawable area, or when the
// draw limit ends the loop before any draw landed outside the banned
// territory.
func (m *Maker) RandomLocation(geo Terrain, excludeBanned bool) (loc location.Location, ok bool) {
	if m == nil {
		return location.Location{}, false
	}
	minZ, maxZ, hasZ := m.MergedZRange()
	triangles := m.Triangles()
	var total int64
	for _, tri := range triangles {
		total += tri.Size()
	}
	if !hasZ || total <= 0 {
		return location.Location{}, false
	}
	avgZ := (minZ + maxZ) / 2

	failed := 0
	for draw := 0; failed < randomLocationAttempts && draw < randomLocationDrawLimit; draw++ {
		tri := pickTriangle(triangles, int64(rnd.Get(int(total))))
		pt := tri.PointAt(rnd.GetFloat(1), rnd.GetFloat(1))
		candidate := location.Location{X: pt.X, Y: pt.Y, Z: int(geo.Height(pt.X, pt.Y, avgZ))}
		if excludeBanned && m.ContainsBanned(candidate) {
			continue
		}
		loc, ok = candidate, true
		if loc.Z < minZ || loc.Z > maxZ || !geo.Walkable(loc.X, loc.Y, loc.Z) {
			failed++
			continue
		}
		return loc, true
	}
	return loc, ok
}

// pickTriangle walks triangles subtracting each size from roll, a value in
// [0, total size), and returns the one that takes it below zero.
func pickTriangle(triangles []geometry.Triangle, roll int64) geometry.Triangle {
	for _, tri := range triangles {
		roll -= tri.Size()
		if roll < 0 {
			return tri
		}
	}
	return triangles[len(triangles)-1]
}
