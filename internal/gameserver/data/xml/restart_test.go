package xml

import (
	"path/filepath"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
)

func TestLoadRestartPoints(t *testing.T) {
	path := datapackPath(t, filepath.Join("data", "xml", "restartPointAreas.xml"))

	table, err := LoadRestartPoints(path)
	if err != nil {
		t.Fatalf("LoadRestartPoints(%q) error: %v", path, err)
	}

	if got, want := len(table.Areas), 92; got != want {
		t.Fatalf("len(Areas) = %d, want %d", got, want)
	}
	if got, want := len(table.Points), 36; got != want {
		t.Fatalf("len(Points) = %d, want %d", got, want)
	}

	area := table.Areas[0]
	if area.MinZ != -3596 || area.MaxZ != -3396 || len(area.Nodes) != 4 {
		t.Fatalf("first area = %+v", area)
	}
	if got := area.Restrictions[player.RaceHuman]; got != "monster_race" {
		t.Fatalf("first area human restriction = %q, want monster_race", got)
	}

	point := table.Points[0]
	if point.Name != "talking_island_town" || point.BBS != 1 || point.LocName != 910 {
		t.Fatalf("first point = %+v", point)
	}
	if len(point.Points) != 14 || len(point.ChaoPoints) != 20 || len(point.MapRegions) != 9 {
		t.Fatalf("first point list lengths = points=%d chao=%d maps=%d", len(point.Points), len(point.ChaoPoints), len(point.MapRegions))
	}

	var banned bool
	for _, p := range table.Points {
		if p.HasBannedRace && p.BannedRace == player.RaceDarkElf && p.BannedPoint == "darkelf_town" {
			banned = true
		}
	}
	if !banned {
		t.Fatal("dark elf banned restart point not loaded")
	}
}

func TestLoadRestartPointsErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "restartPointAreas.xml")
	writeXMLFixture(t, path, `<list><area minZ="0" maxZ="1"><node x="1" y="2"/><restart race="ALIEN" zone="x"/></area></list>`)

	if _, err := LoadRestartPoints(path); err == nil {
		t.Fatal("LoadRestartPoints() error = nil, want error")
	}
}
