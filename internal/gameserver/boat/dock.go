package boat

import (
	"math"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/route"
)

// Boarding ratios: how far along the way to a dock line a boarding point
// lies, from aboard (past the line) and from the shore (short of it).
const (
	inBoatRatio  = 1.2
	outBoatRatio = 0.9
)

// Dock is where a boat ties up: its tie-up point, the shore point a
// passenger is put back on, and the lines a passenger crosses to board
// (entrance) or leave (exit) while the boat is tied up there.
//
// The deck has its own coordinates, in which a passenger's position is
// kept. axis is the deck line matching the entrance: the deck maps onto the
// world by the rotation and scale that turn axis into the entrance, moved
// to the entrance's first point.
//
// An exclusive dock holds one boat at a time: a boat bound for it waits off
// shore while another is tied up there, and every exclusive dock starts the
// server held.
type Dock struct {
	at        location.Location
	oust      location.Location
	entrance  line
	exit      line
	axis      line
	exclusive bool
	angle     float64
	factor    float64
}

func newDock(at, oust location.Location, entrance, exit, axis line, exclusive bool) *Dock {
	d := &Dock{at: at, oust: oust, entrance: entrance, exit: exit, axis: axis, exclusive: exclusive}
	e, a := entrance.vector(), axis.vector()
	d.angle = e.angleTo(a)
	d.factor = e.length() / a.length()
	return d
}

var dockSites = map[route.Dock]*Dock{
	route.DockTalkingIsland: newDock(location.Location{X: -96622, Y: 261660, Z: -3610}, location.Location{X: -96777, Y: 258970, Z: -3623},
		line{-96622, 261420, -96862, 261420}, line{-96622, 261900, -96862, 261900}, line{230, 0, 230, -260}, true),
	route.DockGludin: newDock(location.Location{X: -95686, Y: 150514, Z: -3610}, location.Location{X: -90015, Y: 150422, Z: -3610},
		line{-95446, 150522, -95454, 150762}, line{-95926, 150506, -95934, 150746}, line{-230, 0, -230, -260}, true),
	route.DockRune: newDock(location.Location{X: 34381, Y: -37680, Z: -3610}, location.Location{X: 34513, Y: -38009, Z: -3640},
		line{34548, -37853, 34375, -38019}, line{34214, -37507, 34042, -37674}, line{230, 0, 230, -260}, true),
	route.DockGiran: newDock(location.Location{X: 48950, Y: 190613, Z: -3610}, location.Location{X: 46763, Y: 187041, Z: -3451},
		line{48845, 190397, 49060, 190292}, line{49055, 190829, 49271, 190723}, line{-230, 0, -230, -260}, false),
	route.DockPrimeval: newDock(location.Location{X: 10342, Y: -27279, Z: -3610}, location.Location{X: 10447, Y: -24982, Z: -3664},
		line{10395, -27034, 10538, -27035}, line{10342, -27519, 10582, -27519}, line{230, 0, 230, -260}, false),
	route.DockInnadril: newDock(location.Location{X: 111384, Y: 226232, Z: -3610}, location.Location{X: 107092, Y: 219098, Z: -3952},
		line{111137, 225989, 111387, 225991}, line{111384, 226472, 111144, 226472}, line{230, -260, 230, 0}, false),
}

// OustLocation is the shore point a passenger of a boat tied up here is put
// back on.
func (d *Dock) OustLocation() location.Location { return d.oust }

// BoardingPoint is where a walk from origin toward dest crosses the dock's
// entrance, moved along that walk by the boarding ratio of a passenger
// (inBoat) or of someone ashore; ok is false when the walk does not cross
// the entrance.
func (d *Dock) BoardingPoint(origin, dest Point, inBoat bool) (Point, bool) {
	return d.entrance.adjustedIntersection(origin, dest, boardingRatio(inBoat))
}

// ExitPoint is BoardingPoint for the dock's exit line.
func (d *Dock) ExitPoint(origin, dest Point, inBoat bool) (Point, bool) {
	return d.exit.adjustedIntersection(origin, dest, boardingRatio(inBoat))
}

// AdjustedBoardingPoint is BoardingPoint, or, when the walk does not cross
// the entrance, the point the boarding ratio puts along the walk itself.
func (d *Dock) AdjustedBoardingPoint(origin, dest Point, inBoat bool) Point {
	if p, ok := d.BoardingPoint(origin, dest, inBoat); ok {
		return p
	}
	ratio := boardingRatio(inBoat)
	return Point{
		X: javaRound(float64(origin.X) + ratio*float64(dest.X-origin.X)),
		Y: javaRound(float64(origin.Y) + ratio*float64(dest.Y-origin.Y)),
	}
}

// BoatToWorld maps deck point (x, y) of a boat tied up here onto the world.
func (d *Dock) BoatToWorld(x, y int) Point {
	p := Point{X: x - d.axis.x1, Y: y - d.axis.y1}.rotate(d.angle).scale(d.factor)
	return Point{X: d.entrance.x1 + p.X, Y: d.entrance.y1 + p.Y}
}

// WorldToBoat maps world point (x, y) onto the deck of a boat tied up here.
func (d *Dock) WorldToBoat(x, y int) Point {
	p := Point{X: x - d.entrance.x1, Y: y - d.entrance.y1}.rotate(-d.angle).scale(1 / d.factor)
	return Point{X: d.axis.x1 + p.X, Y: d.axis.y1 + p.Y}
}

func boardingRatio(inBoat bool) float64 {
	if inBoat {
		return inBoatRatio
	}
	return outBoatRatio
}

// docks tracks which exclusive docks a boat holds. Docks are shared between
// itineraries (Gludin and Rune each serve two), so one docks value serves a
// whole fleet. It is owned by the fleet's tick goroutine.
type docks struct {
	held map[route.Dock]bool
}

func newDocks() *docks { return &docks{held: make(map[route.Dock]bool)} }

// busy reports whether d is an exclusive dock a boat holds.
func (s *docks) busy(d route.Dock) bool {
	return dockSites[d].exclusive && s.held[d]
}

// setBusy marks d held or free. A dock that is not exclusive never counts as
// held.
func (s *docks) setBusy(d route.Dock, held bool) {
	if dockSites[d].exclusive {
		s.held[d] = held
	}
}

// Point is a point on the ground plane.
type Point struct{ X, Y int }

// Distance returns the straight-line distance from p to q.
func (p Point) Distance(q Point) float64 {
	dx, dy := float64(p.X-q.X), float64(p.Y-q.Y)
	return math.Sqrt(dx*dx + dy*dy)
}

func (p Point) length() float64 { return math.Sqrt(float64(p.X*p.X + p.Y*p.Y)) }

// angleTo is the angle from q's direction to p's, counter-clockwise.
func (p Point) angleTo(q Point) float64 {
	return math.Atan2(float64(p.Y), float64(p.X)) - math.Atan2(float64(q.Y), float64(q.X))
}

// rotate turns p counter-clockwise about the origin by angle, rounding each
// coordinate.
func (p Point) rotate(angle float64) Point {
	cos, sin := math.Cos(angle), math.Sin(angle)
	x, y := float64(p.X), float64(p.Y)
	return Point{X: javaRound(x*cos - y*sin), Y: javaRound(x*sin + y*cos)}
}

// scale multiplies p by factor, rounding each coordinate.
func (p Point) scale(factor float64) Point {
	return Point{X: javaRound(float64(p.X) * factor), Y: javaRound(float64(p.Y) * factor)}
}

// javaRound rounds half up, toward positive infinity.
func javaRound(v float64) int { return int(math.Floor(v + 0.5)) }

// line is a segment from (x1, y1) to (x2, y2).
type line struct{ x1, y1, x2, y2 int }

// vector is the segment's run from its first point to its second.
func (l line) vector() Point { return Point{X: l.x2 - l.x1, Y: l.y2 - l.y1} }

// intersection is where l and the segment from start to end cross; ok is
// false when they are parallel or do not meet within both segments.
func (l line) intersection(start, end Point) (Point, bool) {
	dx1, dy1 := float64(l.x2-l.x1), float64(l.y2-l.y1)
	dx2, dy2 := float64(end.X-start.X), float64(end.Y-start.Y)
	delta := dx1*dy2 - dy1*dx2
	if delta == 0 {
		return Point{}, false
	}
	t := (float64(l.y1-start.Y)*dx2 - float64(l.x1-start.X)*dy2) / delta
	u := (float64(l.y1-start.Y)*dx1 - float64(l.x1-start.X)*dy1) / delta
	if t < 0 || t > 1 || u < 0 || u > 1 {
		return Point{}, false
	}
	return Point{X: int(float64(l.x1) + t*dx1), Y: int(float64(l.y1) + t*dy1)}, true
}

// adjustedIntersection moves the crossing of l and the walk from start to
// end along the walk by ratio, measured from start.
func (l line) adjustedIntersection(start, end Point, ratio float64) (Point, bool) {
	p, ok := l.intersection(start, end)
	if !ok {
		return Point{}, false
	}
	return Point{
		X: javaRound(float64(start.X) + ratio*float64(p.X-start.X)),
		Y: javaRound(float64(start.Y) + ratio*float64(p.Y-start.Y)),
	}, true
}
