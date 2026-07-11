package restart

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// Fixture values below are taken directly from data/xml/restartPointAreas.xml
// (the monster-race arena area/point and the elf/dark-elf town pair), so the
// map-region arithmetic and area/point resolution are checked against real,
// self-consistent data rather than invented coordinates.

func monsterRaceArea(t *testing.T) Area {
	t.Helper()
	nodes := []location.Point{
		{X: 11712, Y: 181566}, {X: 14407, Y: 181566}, {X: 14407, Y: 182676}, {X: 11712, Y: 182676},
	}
	restrictions := map[player.Race]string{
		player.RaceHuman: "monster_race", player.RaceElf: "monster_race",
		player.RaceDarkElf: "monster_race", player.RaceOrc: "monster_race", player.RaceDwarf: "monster_race",
	}
	area, err := NewArea(nodes, -3596, -3396, restrictions)
	if err != nil {
		t.Fatalf("NewArea() error: %v", err)
	}
	return area
}

func fixtureTable(t *testing.T) *Table {
	t.Helper()
	return &Table{
		Areas: []Area{monsterRaceArea(t)},
		Points: []Point{
			{
				Name:       "talking_island_town",
				Points:     []location.Location{{X: -83990, Y: 243336, Z: -3700}},
				ChaoPoints: []location.Location{{X: -79411, Y: 240677, Z: -3450}},
				MapRegions: []location.Point{{X: 17, Y: 25}},
				BBS:        1, LocName: 910,
			},
			{
				Name:       "monster_race",
				Points:     []location.Location{{X: 11892, Y: 181700, Z: -3560}},
				ChaoPoints: []location.Location{{X: 11892, Y: 181700, Z: -3560}},
			},
			{
				Name:          "elf_town",
				Points:        []location.Location{{X: 45487, Y: 49767, Z: -2950}, {X: 45080, Y: 49614, Z: -2950}},
				ChaoPoints:    []location.Location{{X: 40720, Y: 56064, Z: -3580}},
				MapRegions:    []location.Point{{X: 21, Y: 19}, {X: 21, Y: 20}, {X: 20, Y: 19}, {X: 20, Y: 20}},
				HasBannedRace: true, BannedRace: player.RaceDarkElf, BannedPoint: "darkelf_town",
				BBS: 4, LocName: 914,
			},
			{
				Name:       "darkelf_town",
				Points:     []location.Location{{X: 11632, Y: 16812, Z: -4500}},
				ChaoPoints: []location.Location{{X: 3040, Y: 24240, Z: -3700}},
			},
		},
	}
}

func TestAreaAt(t *testing.T) {
	table := fixtureTable(t)

	if _, ok := table.AreaAt(location.Location{X: 13000, Y: 182000, Z: -3500}); !ok {
		t.Fatal("AreaAt() = false inside the arena bounds, want true")
	}
	if _, ok := table.AreaAt(location.Location{X: 0, Y: 0, Z: 0}); ok {
		t.Fatal("AreaAt() = true at the world origin, want false")
	}
}

func TestPointAt(t *testing.T) {
	table := fixtureTable(t)

	// talking_island_town's own spawn coordinate maps back to its listed
	// region (17;25) via the same arithmetic the restart XML encodes.
	p, ok := table.PointAt(location.Location{X: -83990, Y: 243336, Z: -3700})
	if !ok || p.Name != "talking_island_town" {
		t.Fatalf("PointAt() = (%+v, %v), want talking_island_town", p, ok)
	}

	if _, ok := table.PointAt(location.Location{X: 0, Y: 0, Z: 0}); ok {
		t.Fatal("PointAt() = true at the world origin, want false")
	}
}

func TestCalculatedPoint(t *testing.T) {
	table := fixtureTable(t)

	// Inside the monster-race arena, the area's class restriction wins over
	// the region's own restart point regardless of race.
	p, ok := table.CalculatedPoint(location.Location{X: 13000, Y: 182000, Z: -3500}, player.RaceHuman)
	if !ok || p.Name != "monster_race" {
		t.Fatalf("CalculatedPoint() in arena = (%+v, %v), want monster_race", p, ok)
	}

	elfTownLoc := location.Location{X: 45487, Y: 49767, Z: -2950}

	// Outside any area, a non-banned race resolves straight to the region's point.
	p, ok = table.CalculatedPoint(elfTownLoc, player.RaceHuman)
	if !ok || p.Name != "elf_town" {
		t.Fatalf("CalculatedPoint() human = (%+v, %v), want elf_town", p, ok)
	}

	// A banned race redirects to the point's alternate.
	p, ok = table.CalculatedPoint(elfTownLoc, player.RaceDarkElf)
	if !ok || p.Name != "darkelf_town" {
		t.Fatalf("CalculatedPoint() dark elf = (%+v, %v), want darkelf_town", p, ok)
	}

	if _, ok := table.CalculatedPoint(location.Location{X: 0, Y: 0, Z: 0}, player.RaceHuman); ok {
		t.Fatal("CalculatedPoint() = true at the world origin, want false")
	}
}

func TestNearestLocation(t *testing.T) {
	table := fixtureTable(t)
	elfTownLoc := location.Location{X: 45487, Y: 49767, Z: -2950}

	// No karma: picks from the regular point list.
	loc, ok := table.NearestLocation(elfTownLoc, player.RaceHuman, 0)
	if !ok {
		t.Fatal("NearestLocation() ok = false, want true")
	}
	if loc != table.Points[2].Points[0] && loc != table.Points[2].Points[1] {
		t.Fatalf("NearestLocation() = %+v, want a member of elf_town's regular points", loc)
	}

	// Positive karma: picks from the chaotic list (elf_town has only one entry).
	loc, ok = table.NearestLocation(elfTownLoc, player.RaceHuman, 5)
	if !ok || loc != (location.Location{X: 40720, Y: 56064, Z: -3580}) {
		t.Fatalf("NearestLocation() karma = (%+v, %v), want elf_town's chaotic point", loc, ok)
	}

	if _, ok := table.NearestLocation(location.Location{X: 0, Y: 0, Z: 0}, player.RaceHuman, 0); ok {
		t.Fatal("NearestLocation() ok = true at the world origin, want false")
	}
}
