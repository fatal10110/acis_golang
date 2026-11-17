package xml

import (
	"path/filepath"
	"testing"
)

// The soul-crystal leveling rows the reference accepts: absorbType is one of
// its three constants, written exactly, and skill is read as a boolean (true
// in any case, anything else false). Every attribute but skill must be there.
func TestLoadSoulCrystalDataNPCRows(t *testing.T) {
	t.Parallel()
	type want struct {
		absorb string
		skill  bool
	}
	for _, tc := range []struct {
		name  string
		attrs string
		want  want
	}{
		{"last hit", `absorbType="LAST_HIT" skill="false"`, want{"LAST_HIT", false}},
		{"full party", `absorbType="FULL_PARTY" skill="true"`, want{"FULL_PARTY", true}},
		{"party one random", `absorbType="PARTY_ONE_RANDOM" skill="True"`, want{"PARTY_ONE_RANDOM", true}},
		{"skill read as boolean", `absorbType="LAST_HIT" skill="yes"`, want{"LAST_HIT", false}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "soulCrystals.xml")
			writeXMLFixture(t, path, `<list><npcs><npc id="22215" chanceStage="30" chanceBreak="5" `+tc.attrs+` levelList="10;12"/></npcs></list>`)
			table, err := LoadSoulCrystalData(path)
			if err != nil {
				t.Fatalf("LoadSoulCrystalData: %v", err)
			}
			info, ok := table.LevelingInfo(22215)
			if !ok || info.AbsorbType != tc.want.absorb || info.SkillRequired != tc.want.skill ||
				info.ChanceStage != 30 || info.ChanceBreak != 5 || len(info.Levels) != 2 || info.Levels[1] != 12 {
				t.Fatalf("LevelingInfo(22215) = %+v, %v; want %+v with chances 30/5 and levels 10;12", info, ok, tc.want)
			}
		})
	}
}

// Each required attribute of a leveling row, absent, fails the load.
func TestLoadSoulCrystalDataNPCRowMissingAttribute(t *testing.T) {
	t.Parallel()
	full := map[string]string{
		"id": "22215", "chanceStage": "30", "chanceBreak": "5", "absorbType": "LAST_HIT", "levelList": "10;12",
	}
	for missing := range full {
		t.Run(missing, func(t *testing.T) {
			t.Parallel()
			attrs := ""
			for k, v := range full {
				if k != missing {
					attrs += k + `="` + v + `" `
				}
			}
			path := filepath.Join(t.TempDir(), "soulCrystals.xml")
			writeXMLFixture(t, path, `<list><npcs><npc `+attrs+`/></npcs></list>`)
			if _, err := LoadSoulCrystalData(path); err == nil {
				t.Fatalf("LoadSoulCrystalData without %s succeeded, want an error", missing)
			}
		})
	}
}
