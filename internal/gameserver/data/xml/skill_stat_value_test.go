package xml

import (
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
)

// TestSkillStatValueMustNameAStat covers #3669: a level's "stat" set value
// is read as a stat constant name exactly (StatSet.getEnum -> Enum.valueOf
// in the skill constructor), so a value naming no stat fails that level,
// which drops with the later levels of its route. An absent value names no
// stat.
func TestSkillStatValueMustNameAStat(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeXMLFixture(t, filepath.Join(dir, "fixture.xml"), `<list><skill id="8" name="x" levels="3">`+abortSets+
		`<table name="#stat"> MAX_HP maxHp POWER_ATTACK </table><set name="stat" val="#stat"/></skill>`+
		`<skill id="9" name="y" levels="1">`+abortSets+`</skill></list>`)
	table, err := LoadSkillDefinitions(dir, zerolog.Nop())
	if err != nil {
		t.Fatalf("LoadSkillDefinitions: %v", err)
	}
	if def, ok := table.Get(8, 1); !ok || def.Stat != "MAX_HP" {
		t.Fatalf("skill 8 level 1 = %v (stat %q), want loaded naming MAX_HP", ok, def.Stat)
	}
	for _, level := range []int{2, 3} {
		if _, ok := table.Get(8, level); ok {
			t.Fatalf("skill 8 level %d loaded, want dropped: level 2's stat names no stat", level)
		}
	}
	if def, ok := table.Get(9, 1); !ok || def.Stat != "" {
		t.Fatalf("skill 9 level 1 = %v (stat %q), want loaded naming no stat", ok, def.Stat)
	}
}
