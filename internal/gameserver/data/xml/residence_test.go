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

// TestLoadCastlesControlTowerLastPositionAndStatsWin covers a control tower
// with more than one <position>/<stats> child: the removed StatSet path
// built the tower's attrs by merging every child in document order, so a
// later child overwrote an earlier one. The XML-tag decode must reproduce
// that last-wins order, not read the first child.
func TestLoadCastlesControlTowerLastPositionAndStatsWin(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "castles.xml")
	writeXMLFixture(t, path, `<list><castle id="1" alias="gludio" parentId="0" name="Gludio" circletId="1">
		<controlTowers>
			<controlTower alias="tower1" type="LIFE_CONTROL">
				<position x="1" y="2" z="3"/>
				<position x="10" y="20" z="30"/>
				<stats hp="100" pDef="1" mDef="1"/>
				<stats hp="200" pDef="2" mDef="2"/>
			</controlTower>
		</controlTowers>
		<npcs val="1"/>
		<tax taxRate="0" taxSysgetRate="0" tributeRate="0"/>
	</castle></list>`)

	table, err := LoadCastles(path)
	if err != nil {
		t.Fatalf("LoadCastles(%q) error: %v", path, err)
	}
	c, ok := table.Get(1)
	if !ok {
		t.Fatal("Get(1) returned no castle")
	}
	if len(c.ControlTowers) != 1 {
		t.Fatalf("len(ControlTowers) = %d, want 1", len(c.ControlTowers))
	}
	tower := c.ControlTowers[0]
	if tower.Position.X != 10 || tower.Position.Y != 20 || tower.Position.Z != 30 {
		t.Fatalf("Position = %+v, want last <position> (10,20,30)", tower.Position)
	}
	if tower.HP != 200 || tower.PDef != 2 || tower.MDef != 2 {
		t.Fatalf("stats = {%v %v %v}, want last <stats> (200,2,2)", tower.HP, tower.PDef, tower.MDef)
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

func TestResidenceLoadersMatchJavaEdgeCases(t *testing.T) {
	dir := t.TempDir()
	castlePath := filepath.Join(dir, "castles.xml")
	writeXMLFixture(t, castlePath, `<list>
		<castle id="2" alias="lists" parentId="0" name="Lists" circletId="1"><gates val="gate1"/><gates val="gate2"/><npcs val="1"/><npcs val="2"/><tax taxRate="0" taxSysgetRate="0" tributeRate="0"/><tickets><ticket itemId="1" type="SWORD" stationary="true" npcId="1" maxAmount="1" ssq="NORMAL"/></tickets></castle>
		<castle id="1" alias="first" parentId="0" name="First" circletId="1"><tax taxRate="0" taxSysgetRate="0" tributeRate="0"/></castle>
		<castle id="1" alias="second" parentId="0" name="Second" circletId="1"><tax taxRate="0" taxSysgetRate="0" tributeRate="0"/></castle>
	</list>`)
	castles, err := LoadCastles(castlePath)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := castles.Get(1); !ok || got.Name != "Second" || len(castles.All()) != 2 {
		t.Fatalf("duplicate castle = %#v, all=%d", got, len(castles.All()))
	}
	if got, _ := castles.Get(2); len(got.Gates) != 2 || len(got.NPCs) != 2 {
		t.Fatalf("repeated castle lists = gates=%v npcs=%v", got.Gates, got.NPCs)
	}
	if _, ok := castles.ByAlias("first"); ok {
		t.Fatal("replaced castle retained its old alias")
	}
	if got, _ := castles.ByAlias("second"); got.ID != 1 {
		t.Fatalf("replaced castle alias = %d, want 1", got.ID)
	}
	writeXMLFixture(t, castlePath, `<list><castle id="1" alias="same" parentId="0" name="One" circletId="1"><tax taxRate="0" taxSysgetRate="0" tributeRate="0"/></castle><castle id="2" alias="same" parentId="0" name="Two" circletId="1"><tax taxRate="0" taxSysgetRate="0" tributeRate="0"/></castle></list>`)
	if _, err := LoadCastles(castlePath); err == nil {
		t.Fatal("LoadCastles accepted duplicate aliases for different ids")
	}

	castlePath = filepath.Join(dir, "invalid-ticket.xml")
	writeXMLFixture(t, castlePath, `<list><castle id="1" alias="castle" parentId="0" name="Castle" circletId="1"><tax taxRate="0" taxSysgetRate="0" tributeRate="0"/><tickets><ticket itemId="1" type="BAD" stationary="true" npcId="1" maxAmount="1" ssq="NORMAL"/></tickets></castle></list>`)
	if _, err := LoadCastles(castlePath); err == nil {
		t.Fatal("LoadCastles accepted an unknown ticket type")
	}
	writeXMLFixture(t, castlePath, `<list><castle id="1" alias="castle" parentId="0" name="Castle" circletId="1"><tax taxRate="0" taxSysgetRate="0" tributeRate="0"/><tickets><ticket itemId="1" type="SWORD" stationary="true" npcId="1" maxAmount="1" ssq="BAD"/></tickets></castle></list>`)
	if _, err := LoadCastles(castlePath); err == nil {
		t.Fatal("LoadCastles accepted an unknown ticket cabal")
	}

	hallPath := filepath.Join(dir, "clanHalls.xml")
	writeXMLFixture(t, hallPath, `<list>
		<clanHall id="2" alias="lists" parentId="0" name="Lists"><agit desc="Lists" loc="Town"/><gates val="gate1"/><gates val="gate2"/><npcs val="1"/><npcs val="2"/><tax taxRate="0" taxSysgetRate="0" tributeRate="0"/></clanHall>
		<clanHall id="1" alias="first" parentId="0" name="First"><agit desc="First" loc="Town" siegeLength="0" scheduleConfig="1;2;3;4;5"/><tax taxRate="0" taxSysgetRate="0" tributeRate="0"/></clanHall>
		<clanHall id="1" alias="second" parentId="0" name="Second"><agit desc="Second" loc="Town"/><tax taxRate="0" taxSysgetRate="0" tributeRate="0"/></clanHall>
	</list>`)
	halls, err := LoadClanHalls(hallPath)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := halls.Get(1); !ok || got.Name != "Second" || len(halls.All()) != 2 {
		t.Fatalf("duplicate hall = %#v, all=%d", got, len(halls.All()))
	}
	if got, _ := halls.Get(2); len(got.Gates) != 2 || len(got.NPCs) != 2 {
		t.Fatalf("repeated hall lists = gates=%v npcs=%v", got.Gates, got.NPCs)
	}
	if _, ok := halls.ByAlias("first"); ok {
		t.Fatal("replaced hall retained its old alias")
	}
	if got, _ := halls.ByAlias("second"); got.ID != 1 {
		t.Fatalf("replaced hall alias = %d, want 1", got.ID)
	}
	writeXMLFixture(t, hallPath, `<list><clanHall id="1" alias="same" parentId="0" name="One"><agit desc="One" loc="Town"/><tax taxRate="0" taxSysgetRate="0" tributeRate="0"/></clanHall><clanHall id="2" alias="same" parentId="0" name="Two"><agit desc="Two" loc="Town"/><tax taxRate="0" taxSysgetRate="0" tributeRate="0"/></clanHall></list>`)
	if _, err := LoadClanHalls(hallPath); err == nil {
		t.Fatal("LoadClanHalls accepted duplicate aliases for different ids")
	}

	hallPath = filepath.Join(dir, "zero-siege.xml")
	writeXMLFixture(t, hallPath, `<list><clanHall id="1" alias="hall" parentId="0" name="Hall"><agit desc="Hall" loc="Town" siegeLength="0" scheduleConfig="1;2;3;4;5"/><tax taxRate="0" taxSysgetRate="0" tributeRate="0"/></clanHall></list>`)
	halls, err = LoadClanHalls(hallPath)
	if err != nil {
		t.Fatal(err)
	}
	if hall, _ := halls.Get(1); !hall.IsSiegable() {
		t.Fatal("zero siegeLength must still be siegable when the attribute is present")
	}

	decoPath := filepath.Join(dir, "clanHallDeco.xml")
	writeXMLFixture(t, decoPath, `<list><deco depth="1" days="2" price="3"/><deco name="later" type="0" level="0" depth="4" days="5" price="6"/></list>`)
	decos, err := LoadClanHallDeco(decoPath)
	if err != nil {
		t.Fatal(err)
	}
	if decos.Count() != 2 || decos.Fee(0, 0) != 3 {
		t.Fatalf("decos count=%d fee=%d, want 2 and 3", decos.Count(), decos.Fee(0, 0))
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
			name:    "castle tax missing tributeRate",
			path:    filepath.Join(dir, "castles.xml"),
			content: `<list><castle id="1" alias="gludio" parentId="0" name="Gludio" circletId="1"><npcs val="1"/><tax taxRate="15" taxSysgetRate="40"/><spawns><spawn type="OWNER" x="1" y="2" z="3"/></spawns></castle></list>`,
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
