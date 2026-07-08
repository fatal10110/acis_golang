package xml

import (
	"path/filepath"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/residence"
)

func TestLoadCastles(t *testing.T) {
	path := datapackPath(t, filepath.Join("data", "xml", "castles.xml"))

	table, err := LoadCastles(path)
	if err != nil {
		t.Fatalf("LoadCastles(%q) error: %v", path, err)
	}

	if got := table.Len(); got != 9 {
		t.Fatalf("Len() = %d, want 9", got)
	}
	gludio, ok := table.Get(1)
	if !ok {
		t.Fatal("Get(1) returned no castle")
	}
	if gludio.Alias != "gludio_castle" || gludio.CircletID != 6838 || gludio.Tax.TributeRate != 25 {
		t.Fatalf("Get(1) = %+v", gludio)
	}
	if len(gludio.Artifacts) != 1 || gludio.Artifacts[0].NPCID != 35063 || gludio.Artifacts[0].Heading != 16384 {
		t.Fatalf("gludio artifacts = %+v", gludio.Artifacts)
	}
	if len(gludio.ControlTowers) != 5 || gludio.ControlTowers[3].Type.String() != "TRAP_CONTROL" {
		t.Fatalf("gludio control towers = %+v", gludio.ControlTowers)
	}
	if len(gludio.Spawns[residence.SpawnChaotic]) != 20 {
		t.Fatalf("len(gludio chaotic spawns) = %d, want 20", len(gludio.Spawns[residence.SpawnChaotic]))
	}
	if len(gludio.Tickets) != 55 || gludio.Tickets[len(gludio.Tickets)-1].ItemID != 3972 {
		t.Fatalf("gludio tickets = %d, last=%+v", len(gludio.Tickets), gludio.Tickets[len(gludio.Tickets)-1])
	}
	if len(gludio.Zones) != 3 || gludio.Zones[2].Type != residence.ZoneHeadquarter {
		t.Fatalf("gludio zones = %+v", gludio.Zones)
	}
}

func TestLoadClanHalls(t *testing.T) {
	path := datapackPath(t, filepath.Join("data", "xml", "clanHalls.xml"))

	table, err := LoadClanHalls(path)
	if err != nil {
		t.Fatalf("LoadClanHalls(%q) error: %v", path, err)
	}

	if got := table.Len(); got != 44 {
		t.Fatalf("Len() = %d, want 44", got)
	}
	partisan, ok := table.Get(21)
	if !ok {
		t.Fatal("Get(21) returned no hall")
	}
	if !partisan.IsSiegable() || partisan.SiegeLength != 3600000 {
		t.Fatalf("Get(21).SiegeLength = %d, want 3600000", partisan.SiegeLength)
	}
	if len(partisan.ScheduleConfig) != 5 || partisan.ScheduleConfig[0] != 14 || partisan.ScheduleConfig[3] != 12 {
		t.Fatalf("Get(21).ScheduleConfig = %v", partisan.ScheduleConfig)
	}
	if len(partisan.Spawns[residence.SpawnBanish]) != 1 {
		t.Fatalf("len(Get(21).Spawns[BANISH]) = %d, want 1", len(partisan.Spawns[residence.SpawnBanish]))
	}

	moonstone, ok := table.ByAlias("gludio_castle_agit_001")
	if !ok {
		t.Fatal(`ByAlias("gludio_castle_agit_001") returned no hall`)
	}
	if moonstone.Name != "Moonstone Hall" || moonstone.Town != "Gludio" || moonstone.AuctionMin != 20000000 {
		t.Fatalf("moonstone = %+v", moonstone)
	}
}

func TestLoadClanHallDeco(t *testing.T) {
	path := datapackPath(t, filepath.Join("data", "xml", "clanHallDeco.xml"))

	table, err := LoadClanHallDeco(path)
	if err != nil {
		t.Fatalf("LoadClanHallDeco(%q) error: %v", path, err)
	}

	if got := table.Count(); got != 73 {
		t.Fatalf("Count() = %d, want 73", got)
	}
	if got := table.Fee(1, 20); got != 26500 {
		t.Fatalf("Fee(1, 20) = %d, want 26500", got)
	}
	if got := table.Days(12, 12); got != 7 {
		t.Fatalf("Days(12, 12) = %d, want 7", got)
	}
	if got := table.Depth(9, 18); got != 1 {
		t.Fatalf("Depth(9, 18) = %d, want 1", got)
	}
}

func TestResidenceLoadersErrors(t *testing.T) {
	dir := t.TempDir()

	cases := []struct {
		name    string
		path    string
		content string
		load    func(string) error
	}{
		{
			name:    "castle missing tax",
			path:    filepath.Join(dir, "castles.xml"),
			content: `<list><castle id="1" alias="gludio" parentId="0" name="Gludio" circletId="1"><npcs val="1"/><spawns><spawn type="OWNER" x="1" y="2" z="3"/></spawns></castle></list>`,
			load: func(path string) error {
				_, err := LoadCastles(path)
				return err
			},
		},
		{
			name:    "clan hall bad schedule config",
			path:    filepath.Join(dir, "clanHalls.xml"),
			content: `<list><clanHall id="21" alias="hall" parentId="0" name="Hall"><agit desc="Contestable Clan Hall" loc="Dion" siegeLength="3600000" scheduleConfig="14;bad;0;12;0" auctionMin="0" deposit="0" lease="0" size="0" grade="2"/><npcs val="1"/><tax taxRate="0" taxSysgetRate="0" tributeRate="50"/></clanHall></list>`,
			load: func(path string) error {
				_, err := LoadClanHalls(path)
				return err
			},
		},
		{
			name:    "clan hall deco missing price",
			path:    filepath.Join(dir, "clanHallDeco.xml"),
			content: `<list><deco name="fireplace_12" type="1" level="1" depth="1" days="1"/></list>`,
			load: func(path string) error {
				_, err := LoadClanHallDeco(path)
				return err
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			writeXMLFixture(t, c.path, c.content)
			if err := c.load(c.path); err == nil {
				t.Fatalf("expected an error for %s, got nil", c.name)
			}
		})
	}
}
