package xml

import (
	"path/filepath"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/records"
)

// TestLoadPlayerData compares load counts and field-level values against
// the aCis_datapack classes/*.xml data, which is the same data the Java
// PlayerData loader reads. Expected values below are copied verbatim from
// the XML attributes (id 0, id 1) or computed independently by walking the
// same files with a throwaway script that counts <skill> elements per
// <class> block and follows the documented profession parent chain (id 2,
// id 88) - not re-derived from this package's own code.
func TestLoadPlayerData(t *testing.T) {
	dir := datapackPath(t, filepath.Join("data", "xml", "classes"))

	data, err := LoadPlayerData(dir)
	if err != nil {
		t.Fatalf("LoadPlayerData(%q) error: %v", dir, err)
	}

	const wantCount = 89
	if got := data.Count(); got != wantCount {
		t.Fatalf("Count() = %d, want %d", got, wantCount)
	}

	// Ids 58-87 are reserved by the enum layout and never assigned to a
	// profession in the data; they must not appear as loaded templates.
	for _, reserved := range []int{58, 60, 75, 87} {
		if _, ok := data.Template(reserved); ok {
			t.Errorf("template %d: reserved id must not be loaded", reserved)
		}
	}

	t.Run("HumanFighter base class (id 0)", func(t *testing.T) {
		tmpl, ok := data.Template(0)
		if !ok {
			t.Fatal("template 0 not loaded")
		}

		if tmpl.BaseLevel != 1 {
			t.Errorf("BaseLevel = %d, want 1", tmpl.BaseLevel)
		}
		if tmpl.FistsItemID != 246 {
			t.Errorf("FistsItemID = %d, want 246", tmpl.FistsItemID)
		}
		if tmpl.STR != 40 || tmpl.CON != 43 || tmpl.DEX != 30 || tmpl.INT != 21 || tmpl.WIT != 11 || tmpl.MEN != 25 {
			t.Errorf("base stats = %+v, want STR=40 CON=43 DEX=30 INT=21 WIT=11 MEN=25", tmpl)
		}
		if tmpl.PAtk != 4 || tmpl.PDef != 80 || tmpl.MAtk != 6 || tmpl.MDef != 41 {
			t.Errorf("PAtk/PDef/MAtk/MDef = %v/%v/%v/%v, want 4/80/6/41", tmpl.PAtk, tmpl.PDef, tmpl.MAtk, tmpl.MDef)
		}
		if tmpl.RunSpeed != 115 || tmpl.WalkSpeed != 80 || tmpl.SwimSpeed != 50 {
			t.Errorf("RunSpeed/WalkSpeed/SwimSpeed = %v/%v/%v, want 115/80/50", tmpl.RunSpeed, tmpl.WalkSpeed, tmpl.SwimSpeed)
		}
		if tmpl.CollisionRadius != 9 || tmpl.CollisionRadiusFemale != 8 {
			t.Errorf("CollisionRadius/Female = %v/%v, want 9/8", tmpl.CollisionRadius, tmpl.CollisionRadiusFemale)
		}
		if tmpl.CollisionHeight != 23 || tmpl.CollisionHeightFemale != 23.5 {
			t.Errorf("CollisionHeight/Female = %v/%v, want 23/23.5", tmpl.CollisionHeight, tmpl.CollisionHeightFemale)
		}
		if tmpl.SafeFallHeightFemale != 270 || tmpl.SafeFallHeightMale != 250 {
			t.Errorf("SafeFallHeight Female/Male = %d/%d, want 270/250", tmpl.SafeFallHeightFemale, tmpl.SafeFallHeightMale)
		}

		for _, tc := range []struct {
			name  string
			table []float64
			first float64
			last  float64
		}{
			{"HPTable", tmpl.HPTable, 80, 1415.1},
			{"MPTable", tmpl.MPTable, 30, 646.2},
			{"CPTable", tmpl.CPTable, 32, 566.04},
			{"HPRegenTable", tmpl.HPRegenTable, 2.0, 8.5},
			{"MPRegenTable", tmpl.MPRegenTable, 0.9, 3.0},
			{"CPRegenTable", tmpl.CPRegenTable, 2.0, 8.5},
		} {
			if len(tc.table) != 80 {
				t.Errorf("%s length = %d, want 80", tc.name, len(tc.table))
				continue
			}
			if tc.table[0] != tc.first {
				t.Errorf("%s[0] = %v, want %v", tc.name, tc.table[0], tc.first)
			}
			if tc.table[79] != tc.last {
				t.Errorf("%s[79] = %v, want %v", tc.name, tc.table[79], tc.last)
			}
		}

		wantItems := []records.NewbieItem{
			{ID: 1147, Count: 1, IsEquipped: true},
			{ID: 1146, Count: 1, IsEquipped: true},
			{ID: 10, Count: 1, IsEquipped: false},
			{ID: 2369, Count: 1, IsEquipped: true},
			{ID: 5588, Count: 1, IsEquipped: true},
		}
		if len(tmpl.Items) != len(wantItems) {
			t.Fatalf("Items = %+v, want %+v", tmpl.Items, wantItems)
		}
		for i, want := range wantItems {
			if tmpl.Items[i] != want {
				t.Errorf("Items[%d] = %+v, want %+v", i, tmpl.Items[i], want)
			}
		}

		wantSpawns := []location.Location{
			{X: -71338, Y: 258271, Z: -3104},
			{X: -71417, Y: 258270, Z: -3104},
			{X: -71453, Y: 258305, Z: -3104},
			{X: -71467, Y: 258378, Z: -3104},
		}
		if len(tmpl.SpawnLocations) != len(wantSpawns) {
			t.Fatalf("SpawnLocations = %+v, want %+v", tmpl.SpawnLocations, wantSpawns)
		}
		for i, want := range wantSpawns {
			if tmpl.SpawnLocations[i] != want {
				t.Errorf("SpawnLocations[%d] = %+v, want %+v", i, tmpl.SpawnLocations[i], want)
			}
		}

		if len(tmpl.Skills) != 52 {
			t.Errorf("len(Skills) = %d, want 52 (own skills only, no parent)", len(tmpl.Skills))
		}
	})

	// Skill counts below (own / merged) were computed by an independent
	// script that counts <skill> elements per <class> block and walks the
	// documented parent chain, cross-checked against every id present in
	// the data matching the known profession ids exactly.
	for _, tc := range []struct {
		name           string
		id             int
		wantItems      int
		wantSpawns     int
		wantSkillCount int
	}{
		{"Warrior, tier 1, own+HumanFighter", 1, 0, 0, 156},
		{"Gladiator, tier 2, own+Warrior+HumanFighter", 2, 0, 0, 625},
		{"Duelist, tier 3, own+Gladiator+Warrior+HumanFighter", 88, 0, 0, 639},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tmpl, ok := data.Template(tc.id)
			if !ok {
				t.Fatalf("template %d not loaded", tc.id)
			}
			if len(tmpl.Items) != tc.wantItems {
				t.Errorf("len(Items) = %d, want %d", len(tmpl.Items), tc.wantItems)
			}
			if len(tmpl.SpawnLocations) != tc.wantSpawns {
				t.Errorf("len(SpawnLocations) = %d, want %d", len(tmpl.SpawnLocations), tc.wantSpawns)
			}
			if len(tmpl.Skills) != tc.wantSkillCount {
				t.Errorf("len(Skills) = %d, want %d", len(tmpl.Skills), tc.wantSkillCount)
			}
		})
	}
}

func TestLoadPlayerDataMissingDir(t *testing.T) {
	_, err := LoadPlayerData(filepath.Join(t.TempDir(), "does-not-exist"))
	if err == nil {
		t.Fatal("expected an error for a directory with no *.xml files, got nil")
	}
}

// TestLoadPlayerDataDefaults exercises attribute defaults the real data
// never triggers (every real class line sets swimSpd, and isEquipped is
// only ever set to override the default): swimSpd defaults to 1 and a
// starter item with no isEquipped attribute defaults to equipped.
func TestLoadPlayerDataDefaults(t *testing.T) {
	const doc = `<?xml version='1.0' encoding='utf-8'?>
<list>
	<class>
		<set id="0" baseLvl="1" fists="1"/>
		<set str="1" con="1" dex="1" int="1" wit="1" men="1"/>
		<set pAtk="1" pDef="1" mAtk="1" mDef="1" runSpd="1" walkSpd="1"/>
		<set radius="1" radiusFemale="1"/>
		<set height="1" heightFemale="1"/>
		<set safeFallHeight="1;1"/>
		<set hpTable="1" mpTable="1" cpTable="1"/>
		<set hpRegenTable="1" mpRegenTable="1" cpRegenTable="1"/>
		<items>
			<item id="10" count="1"/>
		</items>
	</class>
</list>`

	dir := t.TempDir()
	writeXMLFixture(t, filepath.Join(dir, "test.xml"), doc)

	data, err := LoadPlayerData(dir)
	if err != nil {
		t.Fatalf("LoadPlayerData error: %v", err)
	}

	tmpl, ok := data.Template(0)
	if !ok {
		t.Fatal("template 0 not loaded")
	}
	if tmpl.SwimSpeed != 1 {
		t.Errorf("SwimSpeed = %d, want default 1", tmpl.SwimSpeed)
	}
	if len(tmpl.Items) != 1 || !tmpl.Items[0].IsEquipped {
		t.Errorf("Items = %+v, want a single equipped-by-default item", tmpl.Items)
	}
}

func TestLoadPlayerDataErrors(t *testing.T) {
	cases := []struct {
		name string
		doc  string
	}{
		{
			// A class id absent from the ClassId parent table must be
			// rejected rather than silently carried without inheritance.
			name: "unknown parent mapping",
			doc: `<?xml version='1.0' encoding='utf-8'?>
<list>
	<class>
		<set id="9001" baseLvl="1" fists="1"/>
		<set str="1" con="1" dex="1" int="1" wit="1" men="1"/>
		<set pAtk="1" pDef="1" mAtk="1" mDef="1" runSpd="1" walkSpd="1" swimSpd="1"/>
		<set radius="1" radiusFemale="1"/>
		<set height="1" heightFemale="1"/>
		<set safeFallHeight="1;1"/>
		<set hpTable="1" mpTable="1" cpTable="1"/>
		<set hpRegenTable="1" mpRegenTable="1" cpRegenTable="1"/>
	</class>
</list>`,
		},
		{
			// Missing required attributes (everything past baseLvl) must
			// surface as a load error.
			name: "missing required attribute",
			doc: `<?xml version='1.0' encoding='utf-8'?>
<list>
	<class>
		<set id="0" baseLvl="1"/>
	</class>
</list>`,
		},
		{
			// swimSpd is optional, but a present-and-malformed value must
			// fail like any other attribute, not silently fall back to the
			// default (mirrors Java's StatSet.getInteger(key, default)).
			name: "malformed optional swimSpd",
			doc: `<?xml version='1.0' encoding='utf-8'?>
<list>
	<class>
		<set id="0" baseLvl="1" fists="1"/>
		<set str="1" con="1" dex="1" int="1" wit="1" men="1"/>
		<set pAtk="1" pDef="1" mAtk="1" mDef="1" runSpd="1" walkSpd="1" swimSpd="oops"/>
		<set radius="1" radiusFemale="1"/>
		<set height="1" heightFemale="1"/>
		<set safeFallHeight="1;1"/>
		<set hpTable="1" mpTable="1" cpTable="1"/>
		<set hpRegenTable="1" mpRegenTable="1" cpRegenTable="1"/>
	</class>
</list>`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeXMLFixture(t, filepath.Join(dir, "test.xml"), tc.doc)
			if _, err := LoadPlayerData(dir); err == nil {
				t.Fatalf("expected an error for %s, got nil", tc.name)
			}
		})
	}
}
