// Package move models a creature's requested movement state.
package move

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/engine"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/pathfind"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// Geo supplies the terrain and pathfinding queries movement resolution needs.
//
// Each method returns a single tier of the 3-tier route resolution a move
// request applies: a straight-line reachability gate (CanMove), a routed
// search to step around obstacles (FindPath), and a partial-progress last
// reachable point when neither succeeds (ValidLocation).
type Geo interface {
	CanMove(ox, oy, oz, tx, ty, tz int) bool
	Height(x, y, z int) int16
	FindPath(origin, target location.Location) (waypoints []location.Location, ok bool)
	ValidLocation(ox, oy, oz, tx, ty, tz int) location.Location
}

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
	if g.Finder == nil {
		return nil, false
	}
	path, _, ok := g.Finder.Find(origin, target)
	return path, ok
}

func (g EngineGeo) ValidLocation(ox, oy, oz, tx, ty, tz int) location.Location {
	return g.Engine.ValidLocation(ox, oy, oz, tx, ty, tz)
}
