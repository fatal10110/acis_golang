package zone

import (
	"math"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/geometry"
)

// MarkPart is the part of a zone's outline a Mark draws.
type MarkPart uint8

const (
	// MarkFloor draws the bottom of the volume.
	MarkFloor MarkPart = iota
	// MarkCeiling draws the top of the volume.
	MarkCeiling
	// MarkEdge is a vertical edge from the floor to the ceiling.
	MarkEdge
)

// Mark is one stroke of a zone's outline: a point at X, Y, Z or, when Line
// is set, a segment from there to X2, Y2, Z2.
type Mark struct {
	Part                MarkPart
	Line                bool
	X, Y, Z, X2, Y2, Z2 int
}

// outlineDepth is how far below the volume's bounds its floor and ceiling
// are drawn.
const outlineDepth = 32

// outlineStep is the distance between the posts drawn around a cylinder.
const outlineStep = 50

// Outline lists the strokes that draw f for a game master, in drawing
// order; nil for a form this package did not build.
//
// A cylinder is a ring of posts outlineStep apart: per post a floor point,
// a ceiling point and the edge between them; a radius too small for one
// step draws nothing. A polygon is, per vertex, the floor and ceiling sides
// to the next vertex and the vertex's edge. A box is its floor, its ceiling
// and its four corner edges, its floor and ceiling being drawn with the far
// x corner set to the near one: each collapses onto the near x side, two
// of its four sides empty, which is the outline clients are shown.
func Outline(f Form) []Mark {
	zf, ok := f.(zoneForm)
	if !ok || len(zf.Shapes) == 0 {
		return nil
	}
	z1, z2 := zf.MinZ-outlineDepth, zf.MaxZ-outlineDepth
	var marks []Mark
	switch s := zf.Shapes[0].(type) {
	case geometry.Rectangle:
		x1, x2, y1, y2 := s.Bounds()
		marks = append(marks, rectangleMarks(MarkFloor, x1, y1, x1, y2, z1)...)
		marks = append(marks, rectangleMarks(MarkCeiling, x1, y1, x1, y2, z2)...)
		for _, c := range [][2]int{{x1, y1}, {x1, y2}, {x2, y1}, {x2, y2}} {
			marks = append(marks, Mark{Part: MarkEdge, Line: true, X: c[0], Y: c[1], Z: z1, X2: c[0], Y2: c[1], Z2: z2})
		}
	case geometry.Circle:
		cx, cy := s.Center()
		rad := s.Radius()
		count := int(2 * math.Pi * float64(rad) / outlineStep)
		angle := 2 * math.Pi / float64(count)
		for i := range count {
			x := int(math.Cos(angle*float64(i))*float64(rad)) + cx
			y := int(math.Sin(angle*float64(i))*float64(rad)) + cy
			marks = append(marks,
				Mark{Part: MarkFloor, X: x, Y: y, Z: z1},
				Mark{Part: MarkCeiling, X: x, Y: y, Z: z2},
				Mark{Part: MarkEdge, Line: true, X: x, Y: y, Z: z1, X2: x, Y2: y, Z2: z2})
		}
	case geometry.Polygon:
		v := s.Vertices()
		for i, p := range v {
			n := v[(i+1)%len(v)]
			marks = append(marks,
				Mark{Part: MarkFloor, Line: true, X: p.X, Y: p.Y, Z: z1, X2: n.X, Y2: n.Y, Z2: z1},
				Mark{Part: MarkCeiling, Line: true, X: p.X, Y: p.Y, Z: z2, X2: n.X, Y2: n.Y, Z2: z2},
				Mark{Part: MarkEdge, Line: true, X: p.X, Y: p.Y, Z: z1, X2: p.X, Y2: p.Y, Z2: z2})
		}
	}
	return marks
}

// rectangleMarks draws the rectangle with corners x, y and x2, y2 at height
// z as four sides: x,y to x,y2; x2,y2 to x,y2; x2,y2 to x2,y; x,y to x2,y.
func rectangleMarks(part MarkPart, x, y, x2, y2, z int) []Mark {
	side := func(ax, ay, bx, by int) Mark {
		return Mark{Part: part, Line: true, X: ax, Y: ay, Z: z, X2: bx, Y2: by, Z2: z}
	}
	return []Mark{side(x, y, x, y2), side(x2, y2, x, y2), side(x2, y2, x2, y), side(x, y, x2, y)}
}
