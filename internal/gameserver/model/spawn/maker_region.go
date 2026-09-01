package spawn

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/geometry"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// Contains reports whether loc lies inside this maker's merged territory:
// z in the min-of-mins / max-of-maxes range, and (x, y) in any member
// footprint.
func (m *Maker) Contains(loc location.Location) bool {
	minZ, maxZ, ok := mergedZRange(m.Territories)
	if !ok || loc.Z < minZ || loc.Z > maxZ {
		return false
	}
	for _, t := range m.Territories {
		if t.Contains2D(loc.X, loc.Y) {
			return true
		}
	}
	return false
}

// ContainsBanned reports whether loc lies inside this maker's merged banned
// territory, using the same union/Z-merge rule as Contains.
func (m *Maker) ContainsBanned(loc location.Location) bool {
	minZ, maxZ, ok := mergedZRange(m.BannedTerritories)
	if !ok || loc.Z < minZ || loc.Z > maxZ {
		return false
	}
	for _, t := range m.BannedTerritories {
		if t.Contains2D(loc.X, loc.Y) {
			return true
		}
	}
	return false
}

// ContainingTriangle returns the first triangle, in territory then shape
// order, whose 2D footprint contains (x, y).
func (m *Maker) ContainingTriangle(x, y int) (geometry.Triangle, bool) {
	if m == nil {
		return geometry.Triangle{}, false
	}
	for _, t := range m.Territories {
		shape := t.geometryTerritory()
		if shape == nil {
			continue
		}
		if tri, ok := shape.ContainingTriangle(x, y); ok {
			return tri, true
		}
	}
	return geometry.Triangle{}, false
}

// Triangles returns every triangle in this maker's territories, in
// territory then shape order.
func (m *Maker) Triangles() []geometry.Triangle {
	if m == nil {
		return nil
	}
	var out []geometry.Triangle
	for _, t := range m.Territories {
		if shape := t.geometryTerritory(); shape != nil {
			out = shape.AppendTriangles(out)
		}
	}
	return out
}

// MergedZRange is the min-of-mins / max-of-maxes vertical span across
// territories. ok is false when none are present.
func (m *Maker) MergedZRange() (minZ, maxZ int, ok bool) {
	return mergedZRange(m.Territories)
}

func mergedZRange(territories []*Territory) (minZ, maxZ int, ok bool) {
	first := true
	for _, t := range territories {
		if t == nil {
			continue
		}
		if first {
			minZ, maxZ, first = t.MinZ, t.MaxZ, false
			continue
		}
		minZ = min(minZ, t.MinZ)
		maxZ = max(maxZ, t.MaxZ)
	}
	return minZ, maxZ, !first
}
