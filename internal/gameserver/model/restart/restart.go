package restart

import (
	"fmt"
	"math/rand/v2"
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// Area is a polygon that overrides region-scale restart data by race.
type Area struct {
	shape        zone.Form
	Restrictions map[player.Race]string
}

// NewArea builds an Area from a polygon boundary spanning minZ..maxZ.
func NewArea(nodes []location.Point, minZ, maxZ int, restrictions map[player.Race]string) (Area, error) {
	shape, err := zone.NewPolygon(nodes, minZ, maxZ)
	if err != nil {
		return Area{}, fmt.Errorf("restart: area: %w", err)
	}
	return Area{shape: shape, Restrictions: restrictions}, nil
}

// Contains reports whether loc falls inside the area's boundary.
func (a Area) Contains(loc location.Location) bool {
	return a.shape.Contains(loc.X, loc.Y, loc.Z)
}

// ClassRestriction returns the restart point name overridden for race within
// this area, if any.
func (a Area) ClassRestriction(race player.Race) (string, bool) {
	name, ok := a.Restrictions[race]
	return name, ok
}

// Point is a region-scale restart point.
type Point struct {
	Name         string
	Points       []location.Location
	ChaoPoints   []location.Location
	MapRegions   []location.Point
	BBS, LocName int

	BannedRace    player.Race
	HasBannedRace bool
	BannedPoint   string
}

// NewPoint builds a Point from set.
func NewPoint(set *commons.StatSet) (Point, error) {
	idf := commons.NewFields(set, "restart: point")
	name := idf.String("name")
	if err := idf.Err(); err != nil {
		return Point{}, err
	}

	f := commons.NewFields(set, fmt.Sprintf("restart: point %q", name))
	points := commons.FieldList[location.Location](f, "points")
	chaoPoints := commons.FieldList[location.Location](f, "chaoPoints")
	mapRegions := commons.FieldList[location.Point](f, "mapRegions")
	p := Point{
		Name:       name,
		Points:     append([]location.Location(nil), points...),
		ChaoPoints: append([]location.Location(nil), chaoPoints...),
		MapRegions: append([]location.Point(nil), mapRegions...),
		BBS:        f.Int("bbs"),
		LocName:    f.Int("locName"),
	}
	if f.Has("bannedRace") {
		raw := f.String("bannedRace")
		if race, bannedPoint, err := ParseBannedRace(raw); err != nil {
			f.Fail(err)
		} else {
			p.BannedRace, p.BannedPoint, p.HasBannedRace = race, bannedPoint, true
		}
	}
	if err := f.Err(); err != nil {
		return Point{}, err
	}
	return p, nil
}

// Table stores all restart areas and points.
type Table struct {
	Areas  []Area
	Points []Point
}

// AreaAt returns the first restart area containing loc, if any.
func (t *Table) AreaAt(loc location.Location) (Area, bool) {
	for _, a := range t.Areas {
		if a.Contains(loc) {
			return a, true
		}
	}
	return Area{}, false
}

// PointByName returns the restart point with the given name, if any.
func (t *Table) PointByName(name string) (Point, bool) {
	for _, p := range t.Points {
		if strings.EqualFold(p.Name, name) {
			return p, true
		}
	}
	return Point{}, false
}

// mapRegion converts a world position to its region-scale map coordinate,
// the same coarse grid restart points are grouped by (independent of the
// world's visibility grid).
func mapRegion(loc location.Location) location.Point {
	return location.Point{
		X: (loc.X-world.MinX)/world.TileSize + world.TileXMin,
		Y: (loc.Y-world.MinY)/world.TileSize + world.TileYMin,
	}
}

// PointAt returns the restart point whose map region contains loc, if any.
func (t *Table) PointAt(loc location.Location) (Point, bool) {
	region := mapRegion(loc)
	for _, p := range t.Points {
		for _, r := range p.MapRegions {
			if r == region {
				return p, true
			}
		}
	}
	return Point{}, false
}

// CalculatedPoint resolves the restart point that applies to a creature of
// the given race standing at loc: an area's class restriction takes
// priority over the region's general restart point, and a point banning
// race redirects to its alternate point.
func (t *Table) CalculatedPoint(loc location.Location, race player.Race) (Point, bool) {
	if area, ok := t.AreaAt(loc); ok {
		name, ok := area.ClassRestriction(race)
		if !ok {
			return Point{}, false
		}
		return t.PointByName(name)
	}

	point, ok := t.PointAt(loc)
	if !ok {
		return Point{}, false
	}
	if point.HasBannedRace && point.BannedRace == race {
		return t.PointByName(point.BannedPoint)
	}
	return point, true
}

// NearestLocation resolves the destination a creature of the given race and
// karma lands on when restarting at loc: a random point from the resolved
// restart point's chaotic list when karma is positive, otherwise from its
// regular list.
func (t *Table) NearestLocation(loc location.Location, race player.Race, karma int) (location.Location, bool) {
	point, ok := t.CalculatedPoint(loc, race)
	if !ok {
		return location.Location{}, false
	}
	list := point.Points
	if karma > 0 {
		list = point.ChaoPoints
	}
	if len(list) == 0 {
		return location.Location{}, false
	}
	return list[rand.IntN(len(list))], true
}

// ParseRace resolves a restart XML race name.
func ParseRace(s string) (player.Race, error) {
	switch s {
	case "HUMAN":
		return player.RaceHuman, nil
	case "ELF":
		return player.RaceElf, nil
	case "DARK_ELF":
		return player.RaceDarkElf, nil
	case "ORC":
		return player.RaceOrc, nil
	case "DWARF":
		return player.RaceDwarf, nil
	default:
		return 0, fmt.Errorf("restart: unknown race %q", s)
	}
}

// ParseLocationValue parses "x;y;z".
func ParseLocationValue(raw string) (location.Location, error) {
	parts := strings.Split(raw, ";")
	if len(parts) != 3 {
		return location.Location{}, fmt.Errorf("%q must be formatted x;y;z", raw)
	}
	vals := make([]int, 3)
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return location.Location{}, err
		}
		vals[i] = n
	}
	return location.Location{X: vals[0], Y: vals[1], Z: vals[2]}, nil
}

// ParsePointValue parses "x;y".
func ParsePointValue(raw string) (location.Point, error) {
	x, y, ok := strings.Cut(raw, ";")
	if !ok {
		return location.Point{}, fmt.Errorf("%q must be formatted x;y", raw)
	}
	xn, err := strconv.Atoi(x)
	if err != nil {
		return location.Point{}, err
	}
	yn, err := strconv.Atoi(y)
	if err != nil {
		return location.Point{}, err
	}
	return location.Point{X: xn, Y: yn}, nil
}

// ParseBannedRace parses "RACE;restart_point".
func ParseBannedRace(raw string) (player.Race, string, error) {
	raceName, point, ok := strings.Cut(raw, ";")
	if !ok || point == "" {
		return 0, "", fmt.Errorf("%q must be formatted race;point", raw)
	}
	race, err := ParseRace(raceName)
	if err != nil {
		return 0, "", err
	}
	return race, point, nil
}
