package xml

import (
	"bytes"
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

func TestLoadSpawnlistFixture(t *testing.T) {
	dir := t.TempDir()
	writeXMLFixture(t, filepath.Join(dir, "19_21.xml"), `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="a" minZ="-10" maxZ="10">
		<node x="0" y="0"/>
		<node x="1" y="0"/>
		<node x="1" y="1"/>
	</territory>
	<territory name="b" minZ="-20" maxZ="20">
		<node x="10" y="10"/>
		<node x="11" y="10"/>
		<node x="11" y="11"/>
	</territory>
	<territory name="ban1" minZ="-1" maxZ="1">
		<node x="20" y="20"/>
		<node x="21" y="20"/>
		<node x="21" y="21"/>
	</territory>
	<territory name="ban2" minZ="-2" maxZ="2">
		<node x="30" y="30"/>
		<node x="31" y="30"/>
		<node x="31" y="31"/>
	</territory>
	<npcmaker name="maker_1" territory="a;b" ban="ban1;ban2" event="night" spawnTime="day" maximumNpcs="7">
		<ai type="event_maker">
			<set name="EventName" val="@event_mutant_pig"/>
		</ai>
		<npc id="100" total="2" respawn="1min" respawnRand="5sec" pos="1;2;3;4">
			<privates>
				<private id="200" weight="3" respawn="0sec"/>
			</privates>
			<ai>
				<set name="script" val="abc"/>
			</ai>
		</npc>
		<npc id="101" total="1" respawn="2hour" respawnRand="30min" pos="10;20;30;40;60%;11;21;31;41;40%" dbName="boss_1" dbSaving="DEATH_TIME;PARAMETERS"/>
	</npcmaker>
</list>`)

	table, err := LoadSpawnlist(dir, zerolog.Nop())
	if err != nil {
		t.Fatalf("LoadSpawnlist(%q) error: %v", dir, err)
	}

	if got, want := table.TerritoryCount(), 4; got != want {
		t.Fatalf("TerritoryCount() = %d, want %d", got, want)
	}
	if got, want := table.MakerCount(), 1; got != want {
		t.Fatalf("MakerCount() = %d, want %d", got, want)
	}
	if got, want := table.SpawnCount(), 2; got != want {
		t.Fatalf("SpawnCount() = %d, want %d", got, want)
	}

	maker, ok := table.Maker("maker_1")
	if !ok {
		t.Fatal("Maker(maker_1) = missing")
	}
	if got, want := maker.AIType, "event_maker"; got != want {
		t.Fatalf("maker.AIType = %q, want %q", got, want)
	}
	if got, want := maker.AIParams["EventName"], "event_mutant_pig"; got != want {
		t.Fatalf("maker.AIParams[EventName] = %q, want %q", got, want)
	}
	if got, want := len(maker.Territories), 2; got != want {
		t.Fatalf("len(maker.Territories) = %d, want %d", got, want)
	}
	if got, want := len(maker.BannedTerritories), 2; got != want {
		t.Fatalf("len(maker.BannedTerritories) = %d, want %d", got, want)
	}
	if got, want := maker.MaximumNPCs, 7; got != want {
		t.Fatalf("maker.MaximumNPCs = %d, want %d", got, want)
	}
	if got, want := maker.SpawnTime, "day"; got != want {
		t.Fatalf("maker.SpawnTime = %q, want %q", got, want)
	}

	first := maker.Entries[0]
	if got, want := first.RespawnDelay, time.Minute; got != want {
		t.Fatalf("first.RespawnDelay = %v, want %v", got, want)
	}
	if got, want := first.RespawnRandom, 5*time.Second; got != want {
		t.Fatalf("first.RespawnRandom = %v, want %v", got, want)
	}
	if got, want := len(first.Positions), 1; got != want {
		t.Fatalf("len(first.Positions) = %d, want %d", got, want)
	}
	if got, want := first.Positions[0].Heading, 4; got != want {
		t.Fatalf("first.Positions[0].Heading = %d, want %d", got, want)
	}
	if got, want := first.AIParams["script"], "abc"; got != want {
		t.Fatalf("first.AIParams[script] = %q, want %q", got, want)
	}
	if got, want := len(first.Privates), 1; got != want {
		t.Fatalf("len(first.Privates) = %d, want %d", got, want)
	}

	second := maker.Entries[1]
	if got, want := len(second.Positions), 2; got != want {
		t.Fatalf("len(second.Positions) = %d, want %d", got, want)
	}
	if got, want := second.Positions[0].Chance, 60; got != want {
		t.Fatalf("second.Positions[0].Chance = %d, want %d", got, want)
	}
	if got, want := second.DBName, "boss_1"; got != want {
		t.Fatalf("second.DBName = %q, want %q", got, want)
	}
	if got, want := len(second.DBSaving), 2; got != want {
		t.Fatalf("len(second.DBSaving) = %d, want %d", got, want)
	}
}

func TestLoadSpawnlistAllowsIdenticalDuplicateTerritory(t *testing.T) {
	dir := t.TempDir()
	writeXMLFixture(t, filepath.Join(dir, "21_24.xml"), `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="same" minZ="-10" maxZ="10">
		<node x="0" y="0"/>
		<node x="10" y="0"/>
		<node x="10" y="10"/>
	</territory>
	<territory name="same" minZ="-10" maxZ="10">
		<node x="0" y="0"/>
		<node x="10" y="0"/>
		<node x="10" y="10"/>
	</territory>
	<npcmaker name="maker" territory="same" maximumNpcs="1">
		<npc id="1" total="1"/>
	</npcmaker>
</list>`)

	table, err := LoadSpawnlist(dir, zerolog.Nop())
	if err != nil {
		t.Fatalf("LoadSpawnlist(%q) error: %v", dir, err)
	}
	if got, want := table.TerritoryCount(), 2; got != want {
		t.Fatalf("TerritoryCount() = %d, want %d", got, want)
	}
	maker, ok := table.Maker("maker")
	if !ok {
		t.Fatal("Maker(maker) = missing")
	}
	if got, want := len(maker.Territories), 1; got != want {
		t.Fatalf("len(maker.Territories) = %d, want %d", got, want)
	}
}

func TestLoadSpawnlistRetainsConflictingDuplicateTerritory(t *testing.T) {
	dir := t.TempDir()
	writeXMLFixture(t, filepath.Join(dir, "21_24.xml"), `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="a" minZ="0" maxZ="1">
		<node x="0" y="0"/>
		<node x="1" y="0"/>
		<node x="1" y="1"/>
	</territory>
	<territory name="a" minZ="0" maxZ="1">
		<node x="0" y="0"/>
		<node x="2" y="0"/>
		<node x="2" y="2"/>
	</territory>
	<npcmaker name="maker" territory="a" maximumNpcs="1">
		<npc id="1" total="1"/>
	</npcmaker>
</list>`)

	table, err := LoadSpawnlist(dir, zerolog.Nop())
	if err != nil {
		t.Fatalf("LoadSpawnlist(%q) error: %v", dir, err)
	}
	if got, want := table.TerritoryCount(), 2; got != want {
		t.Fatalf("TerritoryCount() = %d, want %d", got, want)
	}
	maker, ok := table.Maker("maker")
	if !ok {
		t.Fatal("Maker(maker) = missing")
	}
	if got, want := len(maker.Territories), 1; got != want {
		t.Fatalf("len(maker.Territories) = %d, want %d", got, want)
	}
	if got, want := maker.Territories[0].Nodes[1].X, 1; got != want {
		t.Fatalf("maker.Territories[0].Nodes[1].X = %d, want %d (first-declared territory)", got, want)
	}
}

// unbuildableTerritoryNodes returns a 103-vertex convex ring: SpawnManager's
// ear-clip triangulation checks its loop bound unconditionally after each
// clip, so a polygon finishing on exactly the 100th ear-clip attempt (n=103)
// still fails, the same as one that never converges
// (geometry.TestTriangulateFailsOnPolygonFinishingAtLoopBound).
func unbuildableTerritoryNodes() string {
	var b strings.Builder
	for i := range 103 {
		angle := 2 * math.Pi * float64(i) / 103
		x := int(1000 * math.Cos(angle))
		y := int(1000 * math.Sin(angle))
		fmt.Fprintf(&b, `<node x="%d" y="%d"/>`, x, y)
	}
	return b.String()
}

// TestLoadSpawnlistSkipsUnbuildableTerritory pins issue #2009: SpawnManager
// wraps Territory construction in a per-territory try/catch and keeps
// loading the rest of the spawnlist on failure. A territory whose polygon
// can't be triangulated logs a warning and is dropped instead of aborting
// the load, and an npcmaker referencing only the dropped territory still
// loads with zero resolved territories (matching findTerritory returning
// null), instead of failing itself.
func TestLoadSpawnlistSkipsUnbuildableTerritory(t *testing.T) {
	dir := t.TempDir()
	writeXMLFixture(t, filepath.Join(dir, "19_21.xml"), `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="bad" minZ="0" maxZ="1">`+unbuildableTerritoryNodes()+`</territory>
	<territory name="good" minZ="0" maxZ="1">
		<node x="0" y="0"/>
		<node x="1" y="0"/>
		<node x="1" y="1"/>
	</territory>
	<npcmaker name="maker_good" territory="good" maximumNpcs="1">
		<npc id="1" total="1"/>
	</npcmaker>
	<npcmaker name="maker_bad" territory="bad" maximumNpcs="1">
		<npc id="1" total="1"/>
	</npcmaker>
</list>`)

	var logs bytes.Buffer
	table, err := LoadSpawnlist(dir, zerolog.New(&logs))
	if err != nil {
		t.Fatalf("LoadSpawnlist(%q) error: %v", dir, err)
	}
	if got, want := table.TerritoryCount(), 1; got != want {
		t.Fatalf("TerritoryCount() = %d, want %d (only the buildable territory)", got, want)
	}
	if !strings.Contains(logs.String(), `"bad"`) {
		t.Fatalf("log output = %q, want a warning naming territory \"bad\"", logs.String())
	}

	good, ok := table.Maker("maker_good")
	if !ok {
		t.Fatal("Maker(maker_good) = missing")
	}
	if got, want := len(good.Territories), 1; got != want {
		t.Fatalf("len(maker_good.Territories) = %d, want %d", got, want)
	}

	bad, ok := table.Maker("maker_bad")
	if !ok {
		t.Fatal("Maker(maker_bad) = missing")
	}
	if got, want := len(bad.Territories), 0; got != want {
		t.Fatalf("len(maker_bad.Territories) = %d, want %d (its only territory was skipped)", got, want)
	}
}

// TestLoadSpawnlistMakerToleratesUnknownTerritory pins the resolution half
// of SpawnManager.findTerritory: an npcmaker naming a territory that was
// never declared at all resolves to null rather than failing, so the maker
// still loads with zero territories instead of aborting the whole spawnlist.
func TestLoadSpawnlistMakerToleratesUnknownTerritory(t *testing.T) {
	dir := t.TempDir()
	writeXMLFixture(t, filepath.Join(dir, "19_21.xml"), `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="a" minZ="0" maxZ="1">
		<node x="0" y="0"/>
		<node x="1" y="0"/>
		<node x="1" y="1"/>
	</territory>
	<npcmaker name="maker" territory="missing" maximumNpcs="1">
		<npc id="1" total="1"/>
	</npcmaker>
</list>`)

	var logs bytes.Buffer
	table, err := LoadSpawnlist(dir, zerolog.New(&logs))
	if err != nil {
		t.Fatalf("LoadSpawnlist(%q) error: %v", dir, err)
	}
	maker, ok := table.Maker("maker")
	if !ok {
		t.Fatal("Maker(maker) = missing")
	}
	if got, want := len(maker.Territories), 0; got != want {
		t.Fatalf("len(maker.Territories) = %d, want %d", got, want)
	}
	if !strings.Contains(logs.String(), `"missing"`) {
		t.Fatalf("log output = %q, want a warning naming territory \"missing\"", logs.String())
	}
}

func TestLoadSpawnlistErrors(t *testing.T) {
	tests := []struct {
		name string
		xml  string
	}{
		{
			name: "malformed pos tuple",
			xml: `<?xml version="1.0"?><list>
				<territory name="a" minZ="0" maxZ="1"><node x="0" y="0"/><node x="1" y="0"/><node x="1" y="1"/></territory>
				<npcmaker name="maker" territory="a" maximumNpcs="1"><npc id="1" total="1" pos="1;2;3"/></npcmaker>
			</list>`,
		},
		{
			name: "bad duration",
			xml: `<?xml version="1.0"?><list>
				<territory name="a" minZ="0" maxZ="1"><node x="0" y="0"/><node x="1" y="0"/><node x="1" y="1"/></territory>
				<npcmaker name="maker" territory="a" maximumNpcs="1"><npc id="1" total="1" respawn="oopsmin"/></npcmaker>
			</list>`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeXMLFixture(t, filepath.Join(dir, "19_21.xml"), tc.xml)
			if _, err := LoadSpawnlist(dir, zerolog.Nop()); err == nil {
				t.Fatal("LoadSpawnlist: expected error")
			}
		})
	}
}

func TestLoadSpawnlistAgainstDatapack(t *testing.T) {
	dir := datapackPath(t, filepath.Join("data", "xml", "spawnlist"))

	table, err := LoadSpawnlist(dir, zerolog.Nop())
	if err != nil {
		t.Fatalf("LoadSpawnlist(%q) error: %v", dir, err)
	}

	if got, want := table.TerritoryCount(), 9434; got != want {
		t.Fatalf("TerritoryCount() = %d, want %d", got, want)
	}
	if got, want := table.MakerCount(), 10072; got != want {
		t.Fatalf("MakerCount() = %d, want %d", got, want)
	}
	if got, want := table.SpawnCount(), 30137; got != want {
		t.Fatalf("SpawnCount() = %d, want %d", got, want)
	}

	maker, ok := table.Maker("godard28_2316_09m1")
	if !ok {
		t.Fatal("Maker(godard28_2316_09m1) = missing")
	}
	if got, want := len(maker.Territories), 2; got != want {
		t.Fatalf("len(godard28_2316_09m1.Territories) = %d, want %d", got, want)
	}

	banned, ok := table.Maker("rune14_1916_75m1")
	if !ok {
		t.Fatal("Maker(rune14_1916_75m1) = missing")
	}
	if got, want := len(banned.BannedTerritories), 5; got != want {
		t.Fatalf("len(rune14_1916_75m1.BannedTerritories) = %d, want %d", got, want)
	}

	eventMaker, ok := table.Maker("godard29_npc2316_pg1m1")
	if !ok {
		t.Fatal("Maker(godard29_npc2316_pg1m1) = missing")
	}
	if got, want := eventMaker.AIType, "event_maker"; got != want {
		t.Fatalf("godard29_npc2316_pg1m1.AIType = %q, want %q", got, want)
	}

	raidMaker, ok := table.Maker("godard28_mb2316_01")
	if !ok {
		t.Fatal("Maker(godard28_mb2316_01) = missing")
	}
	if got, want := raidMaker.Entries[0].DBName, "varka_commnder_mos"; got != want {
		t.Fatalf("raidMaker.Entries[0].DBName = %q, want %q", got, want)
	}
	if got, want := raidMaker.Entries[0].RespawnDelay, 36*time.Hour; got != want {
		t.Fatalf("raidMaker.Entries[0].RespawnDelay = %v, want %v", got, want)
	}
}
