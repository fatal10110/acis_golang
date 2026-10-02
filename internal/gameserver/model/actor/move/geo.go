// Package move models a creature's requested movement state.
package move

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/dynamic"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/engine"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/pathfind"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// Geo supplies the terrain and pathfinding queries movement resolution needs.
//
// Each method returns a single tier of the 3-tier route resolution a move
// request applies: a straight-line reachability gate (CanMove), a routed
// search to step around obstacles (FindPath), and a partial-progress last
// reachable point when neither succeeds (ValidLocation). A swimming or
// flying player resolves through the fly queries instead: a straight 3D
// corridor oheight tall (CanFly) and the last point of it a flier reaches
// (ValidFlyLocation), with no routed search.
type Geo interface {
	CanMove(ox, oy, oz, tx, ty, tz int) bool
	Height(x, y, z int) int16
	FindPath(origin, target location.Location) (waypoints []location.Location, ok bool)
	ValidLocation(ox, oy, oz, tx, ty, tz int) location.Location
	CanFly(ox, oy, oz int, oheight float64, tx, ty, tz int) bool
	ValidFlyLocation(ox, oy, oz int, oheight float64, tx, ty, tz int) location.Location
	// Walkable reports whether the exact point is open geodata in every
	// direction (a 3x3 cell block, not just the point itself), the check
	// Territory-random spawn placement retries against.
	Walkable(x, y, z int) bool
}

// pathIntoFinder is a Geo whose routed search writes into caller-owned
// storage instead of allocating its result. CreatureMove uses it when its
// Geo offers it, and copies a plain FindPath result otherwise.
type pathIntoFinder interface {
	FindPathInto(dst []location.Location, origin, target location.Location) ([]location.Location, bool)
}

var _ pathIntoFinder = EngineGeo{}

// EngineGeo wires a geodata engine and a pathfinder to the Geo interface used
// by CreatureMove. The pathfinder may be nil, in which case FindPath always
// reports no route, leaving the engine's straight-line CanMove and the
// ValidLocation fallback to resolve moves alone.
type EngineGeo struct {
	Engine *engine.Engine
	Finder *pathfind.Finder
}

// NewGeo builds a Geo view over engine e and finder f. f may be nil.
func NewGeo(e *engine.Engine, f *pathfind.Finder) Geo {
	return EngineGeo{Engine: e, Finder: f}
}

func (g EngineGeo) CanMove(ox, oy, oz, tx, ty, tz int) bool {
	return g.Engine.CanMove(ox, oy, oz, tx, ty, tz)
}

func (g EngineGeo) Height(x, y, z int) int16 {
	return g.Engine.Height(x, y, z)
}

func (g EngineGeo) FindPath(origin, target location.Location) ([]location.Location, bool) {
	return g.FindPathInto(nil, origin, target)
}

// FindPathInto is FindPath writing the route into dst's storage, which the
// caller owns; EngineGeo is shared by every mover and keeps no buffer.
func (g EngineGeo) FindPathInto(dst []location.Location, origin, target location.Location) ([]location.Location, bool) {
	if g.Finder == nil {
		return dst[:0], false
	}
	path, _, ok := g.Finder.FindInto(dst, origin, target)
	return path, ok
}

func (g EngineGeo) ValidLocation(ox, oy, oz, tx, ty, tz int) location.Location {
	return g.Engine.ValidLocation(ox, oy, oz, tx, ty, tz)
}

func (g EngineGeo) CanFly(ox, oy, oz int, oheight float64, tx, ty, tz int) bool {
	return g.Engine.CanFly(ox, oy, oz, oheight, tx, ty, tz)
}

func (g EngineGeo) ValidFlyLocation(ox, oy, oz int, oheight float64, tx, ty, tz int) location.Location {
	return g.Engine.ValidFlyLocation(ox, oy, oz, oheight, tx, ty, tz)
}

func (g EngineGeo) Walkable(x, y, z int) bool {
	return g.Engine.CanMoveAround(x, y, z)
}

// CanSeeActor reports actor-to-actor line of sight, the query a player's or
// summon's sight checks take from the movement geo they are handed.
func (g EngineGeo) CanSeeActor(ox, oy, oz int, oCollisionHeight float64, tx, ty, tz int, tCollisionHeight float64) bool {
	return g.Engine.CanSeeActor(ox, oy, oz, oCollisionHeight, tx, ty, tz, tCollisionHeight)
}

// CanSeeActorIgnoring is CanSeeActor leaving ignore, the target's own
// geodata object, out of the query.
func (g EngineGeo) CanSeeActorIgnoring(ox, oy, oz int, oCollisionHeight float64, tx, ty, tz int, tCollisionHeight float64, ignore dynamic.Object) bool {
	return g.Engine.CanSeeActorIgnoring(ox, oy, oz, oCollisionHeight, tx, ty, tz, tCollisionHeight, ignore)
}
