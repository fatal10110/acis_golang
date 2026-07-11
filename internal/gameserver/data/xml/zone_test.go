package xml

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
)

// TestLoadZonesDatapack loads the real datapack zone files and compares
// per-kind zone counts against the reference loader's outcome. The
// reference implementation admits every zone element in these files (all
// have valid shapes and node counts), so the expected counts equal the
// per-file element counts, independently derived from the XML.
func TestLoadZonesDatapack(t *testing.T) {
	index, err := LoadZones(datapackPath(t, filepath.Join("data", "xml", "zones")))
	if err != nil {
		t.Fatalf("LoadZones: %v", err)
	}

	wantCounts := map[string]int{
		"arena": 4, "boss": 17, "castleTeleport": 9, "castle": 9,
		"clanHall": 45, "damage": 35, "derbyTrack": 8, "effect": 167,
		"fishing": 43, "hq": 22, "jail": 4, "motherTree": 16,
		"noLanding": 29, "noRestart": 6, "noSummonFriend": 16,
		"olympiad": 22, "peace": 117, "prayer": 11, "script": 4,
		"siege": 15, "swamp": 26, "town": 18, "water": 373,
	}
	gotCounts := map[string]int{
		"arena":          len(zone.OfKind[*zone.Arena](index)),
		"boss":           len(zone.OfKind[*zone.Boss](index)),
		"castleTeleport": len(zone.OfKind[*zone.CastleTeleport](index)),
		"castle":         len(zone.OfKind[*zone.Castle](index)),
		"clanHall":       len(zone.OfKind[*zone.ClanHall](index)),
		"damage":         len(zone.OfKind[*zone.Damage](index)),
		"derbyTrack":     len(zone.OfKind[*zone.DerbyTrack](index)),
		"effect":         len(zone.OfKind[*zone.Effect](index)),
		"fishing":        len(zone.OfKind[*zone.Fishing](index)),
		"hq":             len(zone.OfKind[*zone.HQ](index)),
		"jail":           len(zone.OfKind[*zone.Jail](index)),
		"motherTree":     len(zone.OfKind[*zone.MotherTree](index)),
		"noLanding":      len(zone.OfKind[*zone.NoLanding](index)),
		"noRestart":      len(zone.OfKind[*zone.NoRestart](index)),
		"noSummonFriend": len(zone.OfKind[*zone.NoSummonFriend](index)),
		"olympiad":       len(zone.OfKind[*zone.Olympiad](index)),
		"peace":          len(zone.OfKind[*zone.Peace](index)),
		"prayer":         len(zone.OfKind[*zone.Prayer](index)),
		"script":         len(zone.OfKind[*zone.Script](index)),
		"siege":          len(zone.OfKind[*zone.Siege](index)),
		"swamp":          len(zone.OfKind[*zone.Swamp](index)),
		"town":           len(zone.OfKind[*zone.Town](index)),
		"water":          len(zone.OfKind[*zone.Water](index)),
	}
	total := 0
	for kind, want := range wantCounts {
		if gotCounts[kind] != want {
			t.Errorf("%s zones = %d, want %d", kind, gotCounts[kind], want)
		}
		total += gotCounts[kind]
	}
	if len(index.All()) != 1016 || total != 1016 {
		t.Errorf("total zones = %d (index %d), want 1016", total, len(index.All()))
	}
}

// TestLoadZonesDatapackFields spot-checks parsed settings against values
// pinned from the datapack files themselves.
func TestLoadZonesDatapackFields(t *testing.T) {
	index, err := LoadZones(datapackPath(t, filepath.Join("data", "xml", "zones")))
	if err != nil {
		t.Fatalf("LoadZones: %v", err)
	}

	// Files load in sorted name order, so the first file's dynamic ids
	// start at 1000; arena zones carry no explicit id.
	arenas := zone.OfKind[*zone.Arena](index)
	if arenas[0].ID() != 1000 || arenas[3].ID() != 1003 {
		t.Errorf("arena dynamic ids = %d..%d, want 1000..1003", arenas[0].ID(), arenas[3].ID())
	}

	// The Giran town zone: polygon, town 9, castle 3 (pinned from the
	// town file).
	var giran *zone.Town
	for _, tz := range zone.OfKind[*zone.Town](index) {
		if tz.TownID == 9 {
			giran = tz
			break
		}
	}
	if giran == nil {
		t.Fatal("Giran town zone (townId 9) not loaded")
	}
	if giran.CastleID != 3 || !giran.Peaceful {
		t.Errorf("Giran town: castle %d peaceful %v, want castle 3, peaceful", giran.CastleID, giran.Peaceful)
	}
	if _, ok := giran.Form().(zone.Polygon); !ok {
		t.Errorf("Giran town form is %T, want polygon", giran.Form())
	}
	// A point in central Giran, inside the pinned polygon and z band.
	if !giran.ContainsPoint(82000, 148000, -3450) {
		t.Error("central Giran not inside the Giran town zone")
	}

	// First effect zone in the file: skill 4070 level 1, reuse 6000ms
	// (pinned from the effect file's first entry).
	effects := zone.OfKind[*zone.Effect](index)
	first := effects[0]
	if len(first.Skills) != 1 || first.Skills[0] != (zone.SkillRef{ID: 4070, Level: 1}) {
		t.Errorf("first effect zone skills = %v, want [{4070 1}]", first.Skills)
	}
	if first.ReuseDelay != 6*time.Second {
		t.Errorf("first effect zone reuse = %v, want 6s", first.ReuseDelay)
	}

	// Boss zones carry explicit ids in the 110000 range.
	if _, ok := index.ByID(110000); !ok {
		t.Error("boss zone id 110000 not loaded")
	}

	// Olympiad stadiums carry NORMAL spawn groups.
	for _, oly := range zone.OfKind[*zone.Olympiad](index) {
		if len(oly.Spawn(zone.SpawnNormal)) == 0 {
			t.Errorf("stadium %d has no NORMAL spawns", oly.ID())
		}
	}
}

func TestLoadZonesRejectsMalformedData(t *testing.T) {
	cases := map[string]struct {
		file    string
		content string
	}{
		"unknown kind": {
			file:    "MysteryZone.xml",
			content: `<list><zone shape="Cuboid" minZ="0" maxZ="10"><node x="0" y="0"/><node x="5" y="5"/></zone></list>`,
		},
		"cuboid node count": {
			file:    "PeaceZone.xml",
			content: `<list><zone shape="Cuboid" minZ="0" maxZ="10"><node x="0" y="0"/></zone></list>`,
		},
		"polygon node count": {
			file:    "PeaceZone.xml",
			content: `<list><zone shape="NPoly" minZ="0" maxZ="10"><node x="0" y="0"/><node x="5" y="5"/></zone></list>`,
		},
		"cylinder missing radius": {
			file:    "PeaceZone.xml",
			content: `<list><zone shape="Cylinder" minZ="0" maxZ="10"><node x="0" y="0"/></zone></list>`,
		},
		"missing nodes": {
			file:    "PeaceZone.xml",
			content: `<list><zone shape="Cuboid" minZ="0" maxZ="10"/></list>`,
		},
		"unknown shape": {
			file:    "PeaceZone.xml",
			content: `<list><zone shape="Sphere" minZ="0" maxZ="10"><node x="0" y="0"/></zone></list>`,
		},
		"malformed skill": {
			file:    "EffectZone.xml",
			content: `<list><zone shape="Cuboid" minZ="0" maxZ="10"><stat name="skill" val="4070"/><node x="0" y="0"/><node x="5" y="5"/></zone></list>`,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeXMLFixture(t, filepath.Join(dir, tc.file), tc.content)
			if _, err := LoadZones(dir); err == nil {
				t.Error("LoadZones succeeded, want error")
			}
		})
	}
}

func TestLoadZonesDynamicIDsJumpPerFile(t *testing.T) {
	dir := t.TempDir()
	writeXMLFixture(t, filepath.Join(dir, "ArenaZone.xml"),
		`<list>
			<zone shape="Cuboid" minZ="0" maxZ="10"><node x="0" y="0"/><node x="5" y="5"/></zone>
			<zone shape="Cuboid" minZ="0" maxZ="10"><node x="10" y="10"/><node x="15" y="15"/></zone>
		</list>`)
	writeXMLFixture(t, filepath.Join(dir, "PeaceZone.xml"),
		`<list>
			<zone id="70000" shape="Cuboid" minZ="0" maxZ="10"><node x="0" y="0"/><node x="5" y="5"/></zone>
			<zone shape="Cuboid" minZ="0" maxZ="10"><node x="10" y="10"/><node x="15" y="15"/></zone>
		</list>`)
	index, err := LoadZones(dir)
	if err != nil {
		t.Fatalf("LoadZones: %v", err)
	}

	arenas := zone.OfKind[*zone.Arena](index)
	if arenas[0].ID() != 1000 || arenas[1].ID() != 1001 {
		t.Errorf("first file dynamic ids = %d, %d, want 1000, 1001", arenas[0].ID(), arenas[1].ID())
	}
	peaces := zone.OfKind[*zone.Peace](index)
	// The explicit id is honored and does not advance the counter; the
	// second file's counter jumped to the next thousand.
	if peaces[0].ID() != 70000 || peaces[1].ID() != 2000 {
		t.Errorf("second file ids = %d, %d, want 70000, 2000", peaces[0].ID(), peaces[1].ID())
	}
}

func TestLoadZonesMissingDir(t *testing.T) {
	if _, err := LoadZones(filepath.Join(os.TempDir(), "no-such-zone-dir")); err == nil {
		t.Error("LoadZones on a missing directory succeeded, want error")
	}
}
