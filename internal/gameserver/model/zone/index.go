package zone

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// regionEdge is the side length, in game coordinates, of one world grid
// region, derived from the world bounds so both packages always agree.
const regionEdge = (world.MaxX - world.MinX + 1) / world.RegionsX

// Index holds every loaded zone and attaches each one to the world grid
// regions its footprint touches, so position lookups only scan the zones
// near a point. Build it once at load time; it is immutable afterwards
// and safe for concurrent readers (per-zone occupancy state has its own
// locking).
type Index struct {
	all      []Kind
	byRegion [world.RegionsX][world.RegionsY][]Kind
}

// NewIndex returns an empty Index.
func NewIndex() *Index { return &Index{} }

// Add registers k and attaches it to every region whose rectangle its
// footprint overlaps. Region rectangles span to the start of the next
// region, so a footprint on a boundary attaches to both sides.
func (ix *Index) Add(k Kind) {
	ix.all = append(ix.all, k)
	form := k.Core().Form()
	for rx := 0; rx < world.RegionsX; rx++ {
		x1 := world.MinX + rx*regionEdge
		x2 := x1 + regionEdge
		for ry := 0; ry < world.RegionsY; ry++ {
			y1 := world.MinY + ry*regionEdge
			y2 := y1 + regionEdge
			if form.IntersectsRect(x1, x2, y1, y2) {
				ix.byRegion[rx][ry] = append(ix.byRegion[rx][ry], k)
			}
		}
	}
}

// All returns every zone in load order.
func (ix *Index) All() []Kind { return ix.all }

// ByID returns the first zone with the given id.
func (ix *Index) ByID(id int) (Kind, bool) {
	for _, k := range ix.all {
		if k.Core().ID() == id {
			return k, true
		}
	}
	return nil, false
}

// At returns the zones attached to the region containing (x, y); nil when
// the point falls outside the world or no zone touches that region.
func (ix *Index) At(x, y int) []Kind {
	if x < world.MinX || x > world.MaxX || y < world.MinY || y > world.MaxY {
		return nil
	}
	return ix.byRegion[(x-world.MinX)/regionEdge][(y-world.MinY)/regionEdge]
}

// Revalidate syncs a's zone occupancy against its current position: every
// zone near a is entered or left as needed. Call it whenever an actor
// moves or teleports.
func (ix *Index) Revalidate(a Actor) {
	pos := a.Position()
	for _, k := range ix.At(pos.X, pos.Y) {
		Revalidate(k, a)
	}
}

// RemoveFrom evicts a from every zone attached to the region containing
// (x, y). Call it with the actor's previous position when it leaves a
// region or the world entirely.
func (ix *Index) RemoveFrom(a Actor, x, y int) {
	for _, k := range ix.At(x, y) {
		Remove(k, a)
	}
}

// FindAt returns the first zone of type T whose volume contains
// (x, y, z).
func FindAt[T Kind](ix *Index, x, y, z int) (T, bool) {
	for _, k := range ix.At(x, y) {
		if t, ok := k.(T); ok && k.Core().ContainsPoint(x, y, z) {
			return t, true
		}
	}
	var zero T
	return zero, false
}

// FindAtXY returns the first zone of type T whose footprint contains
// (x, y), probed at each zone's upper z bound.
func FindAtXY[T Kind](ix *Index, x, y int) (T, bool) {
	for _, k := range ix.At(x, y) {
		if t, ok := k.(T); ok && k.Core().ContainsXY(x, y) {
			return t, true
		}
	}
	var zero T
	return zero, false
}

// OfKind returns every zone of type T, in load order.
func OfKind[T Kind](ix *Index) []T {
	var out []T
	for _, k := range ix.all {
		if t, ok := k.(T); ok {
			out = append(out, t)
		}
	}
	return out
}
