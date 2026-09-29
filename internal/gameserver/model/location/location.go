// Package location contains world-coordinate datatypes.
package location

import (
	"math"

	"github.com/fatal10110/acis_golang/internal/commons/rnd"
)

// Location is a 3D (x/y/z) world point.
type Location struct {
	X, Y, Z int
}

// Distance2D returns the flat ground distance between l and other, ignoring
// the Z axis.
func (l Location) Distance2D(other Location) float64 {
	dx := float64(l.X - other.X)
	dy := float64(l.Y - other.Y)
	return math.Hypot(dx, dy)
}

// Distance3D returns the straight-line distance between l and other.
func (l Location) Distance3D(other Location) float64 {
	dx := float64(l.X - other.X)
	dy := float64(l.Y - other.Y)
	dz := float64(l.Z - other.Z)
	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}

// In3DRange reports whether other is within radius units of l, including
// the exact boundary. It is the inclusive check; game range gates (interaction
// distance, cast range, leash and reward ranges) exclude the boundary and use
// In3DRadius instead.
func (l Location) In3DRange(other Location, radius int) bool {
	return In3DRange(l.X, l.Y, l.Z, other.X, other.Y, other.Z, radius)
}

// In3DRange reports whether two coordinate triples are within radius units
// of each other, including the exact boundary. See In3DRadius for the
// strict variant game range gates use.
func In3DRange(ax, ay, az, bx, by, bz, radius int) bool {
	if radius < 0 {
		return false
	}
	dx := int64(ax) - int64(bx)
	dy := int64(ay) - int64(by)
	dz := int64(az) - int64(bz)
	return dx*dx+dy*dy+dz*dz <= int64(radius)*int64(radius)
}

// In3DRadius reports whether other is strictly inside radius units of l.
// The exact boundary is outside.
func (l Location) In3DRadius(other Location, radius int) bool {
	return l.Distance3D(other) < float64(radius)
}

// In3DRadius reports whether two coordinate triples are strictly inside
// radius units of each other. The exact boundary is outside.
func In3DRadius(ax, ay, az, bx, by, bz, radius int) bool {
	return (Location{X: ax, Y: ay, Z: az}).In3DRadius(Location{X: bx, Y: by, Z: bz}, radius)
}

// In2DRadius reports whether other is strictly inside radius units of l on
// the XY plane, ignoring Z. The exact boundary is outside.
func (l Location) In2DRadius(other Location, radius int) bool {
	return l.Distance2D(other) < float64(radius)
}

// In2DRadius reports whether two XY pairs are strictly inside radius units
// of each other. The exact boundary is outside.
func In2DRadius(ax, ay, bx, by, radius int) bool {
	return (Location{X: ax, Y: ay}).In2DRadius(Location{X: bx, Y: by}, radius)
}

// EquidistantPoint returns point index of count points spaced evenly around
// the circle of radius centered on (cx, cy), at height z. Point 0 lies on the
// +X axis and the rest follow counterclockwise; each offset from the center
// is truncated toward zero.
func EquidistantPoint(cx, cy, z, radius, count, index int) Location {
	if index == 0 {
		return Location{X: cx + radius, Y: cy, Z: z}
	}
	angle := 2 * math.Pi / float64(count) * float64(index)
	return Location{
		X: cx + int(math.Cos(angle)*float64(radius)),
		Y: cy + int(math.Sin(angle)*float64(radius)),
		Z: z,
	}
}

// headingScale converts a full-circle angle in degrees to the game's
// heading range (65536 units per circle): 65536 / 360.
const headingScale = 182.04444444444444

// HeadingTo returns the game heading (0..65535) that faces directly from l
// toward other, ignoring the Z axis. l and other equal is a zero heading.
func (l Location) HeadingTo(other Location) int {
	angle := math.Atan2(float64(other.Y-l.Y), float64(other.X-l.X)) * 180 / math.Pi
	if angle < 0 {
		angle += 360
	}
	return int(math.Round(angle * headingScale))
}

// FleeFrom returns the point distance units from l directly away from
// (fromX, fromY) on the XY plane, with Z unchanged. The offset is split
// between the axes so that |dx|+|dy| = distance, and each part is truncated.
// A source on l's own Y line, l itself included, leaves l unchanged.
func (l Location) FleeFrom(fromX, fromY, distance int) Location {
	xDiff := float64(fromX - l.X)
	yDiff := float64(fromY - l.Y)
	if yDiff == 0 {
		// The ratio below is infinite or NaN here, and both axis offsets
		// truncate to zero.
		return l
	}
	ratio := math.Abs(xDiff / yDiff)
	dy := int(float64(distance) / (ratio + 1))
	dx := int(float64(dy) * ratio)
	if xDiff < 0 {
		l.X += dx
	} else {
		l.X -= dx
	}
	if yDiff < 0 {
		l.Y += dy
	} else {
		l.Y -= dy
	}
	return l
}

// AddRandomOffsetBetween adds a polar offset with radius in [minOffset,
// maxOffset] and a uniform heading. Negative or inverted ranges leave l
// unchanged.
func (l Location) AddRandomOffsetBetween(minOffset, maxOffset int) Location {
	if minOffset < 0 || maxOffset < 0 || maxOffset < minOffset {
		return l
	}
	angle := float64(rnd.Get(360)) * math.Pi / 180
	offset := rnd.GetRange(minOffset, maxOffset)
	l.X += int(float64(offset) * math.Cos(angle))
	l.Y += int(float64(offset) * math.Sin(angle))
	return l
}
