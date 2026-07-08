package xml

import (
	"path/filepath"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/travel"
)

func TestLoadTeleports(t *testing.T) {
	path := datapackPath(t, filepath.Join("data", "xml", "teleports.xml"))

	table, err := LoadTeleports(path)
	if err != nil {
		t.Fatalf("LoadTeleports(%q) error: %v", path, err)
	}

	if got, want := len(table), 117; got != want {
		t.Fatalf("len(table) = %d, want %d", got, want)
	}
	if got, want := table.Count(), 1447; got != want {
		t.Fatalf("Count() = %d, want %d", got, want)
	}

	locs := table[30006]
	if len(locs) == 0 {
		t.Fatal("npc 30006 has no teleports")
	}
	first := locs[0]
	if first.Description != "The Village of Gludin" || first.Kind != travel.KindStandard || first.PriceID != 57 || first.PriceCount != 18000 {
		t.Fatalf("npc 30006 first teleport = %+v", first)
	}
	if first.X != -80749 || first.Y != 149834 || first.Z != -3043 {
		t.Fatalf("npc 30006 first teleport location = %+v", first.Location)
	}

	if got := table[30059][0].CastleID; got != 3 {
		t.Fatalf("npc 30059 first CastleID = %d, want 3", got)
	}
}

func TestLoadInstantTeleports(t *testing.T) {
	path := datapackPath(t, filepath.Join("data", "xml", "instantTeleports.xml"))

	table, err := LoadInstantTeleports(path)
	if err != nil {
		t.Fatalf("LoadInstantTeleports(%q) error: %v", path, err)
	}

	if got, want := len(table), 74; got != want {
		t.Fatalf("len(table) = %d, want %d", got, want)
	}
	if got, want := table.Count(), 122; got != want {
		t.Fatalf("Count() = %d, want %d", got, want)
	}

	locs := table[31111]
	if got, want := len(locs), 2; got != want {
		t.Fatalf("len(table[31111]) = %d, want %d", got, want)
	}
	if locs[0].X != 184466 || locs[0].Y != -9022 || locs[0].Z != -5488 {
		t.Fatalf("table[31111][0] = %+v", locs[0])
	}
}

func TestLoadTeleportsErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "teleports.xml")
	writeXMLFixture(t, path, `<list><telPosList npcId="1"><loc desc="bad" priceId="57" priceCount="1" x="1" y="2"/></telPosList></list>`)

	if _, err := LoadTeleports(path); err == nil {
		t.Fatal("LoadTeleports() error = nil, want error")
	}
}
