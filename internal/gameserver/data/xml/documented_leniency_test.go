package xml

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/rs/zerolog"
)

// The two reader rows below are deliberate leniencies, documented at their
// readers (issues #4779 and #4781): the reference rejects the same input.
// They pin what the loader does today so a change shows up here.

// negateStats is split on whitespace runs and kept as written, so a doubled
// separator loads. The reference splits on single spaces and rejects the empty
// token, which drops the level and every later level of its route.
func TestLoadSkillDefinitionsNegateStatsDoubledSeparatorLoads(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeXMLFixture(t, filepath.Join(dir, "fixture.xml"), skillFixture(`<set name="negateStats" val="BUFF  DEBUFF"/>`))
	table, err := LoadSkillDefinitions(dir, zerolog.Nop())
	if err != nil {
		t.Fatalf("LoadSkillDefinitions: %v", err)
	}
	d, ok := table.Get(1, 1)
	if !ok || !slices.Equal(d.Extra().NegateTypes, []string{"BUFF", "DEBUFF"}) {
		t.Fatalf("skill 1 level 1 NegateTypes = %v (loaded %v), want [BUFF DEBUFF]", d.Extra().NegateTypes, ok)
	}
}

// An unknown absorbType loads as written, and a missing skill reads false;
// the reference rejects the first at load and fails the file on the second.
func TestLoadSoulCrystalDataNPCRowLenientAttributes(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		attrs      string
		wantAbsorb string
		wantSkill  bool
	}{
		{"unknown absorb type", `absorbType="NOT_AN_ABSORB_TYPE" skill="true"`, "NOT_AN_ABSORB_TYPE", true},
		{"missing skill", `absorbType="LAST_HIT"`, "LAST_HIT", false},
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
			if !ok || info.AbsorbType != tc.wantAbsorb || info.SkillRequired != tc.wantSkill {
				t.Fatalf("LevelingInfo(22215) = %+v, %v; want absorb %q skill %v", info, ok, tc.wantAbsorb, tc.wantSkill)
			}
		})
	}
}
