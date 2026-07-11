package restart

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// Area is a polygon that overrides region-scale restart data by race.
type Area struct {
	MinZ, MaxZ   int
	Nodes        []location.Point
	Restrictions map[player.Race]string
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
