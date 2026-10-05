package xml

import (
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// loadNPCSkillsFixture loads one npc (id 1) whose <skills> block is body.
func loadNPCSkillsFixture(t *testing.T, body string, table *skill.Table) (*npc.Template, error) {
	t.Helper()
	dir := t.TempDir()
	writeXMLFixture(t, filepath.Join(dir, "fixture.xml"), `<list><npc id="1" name="x">`+npcRequiredSets+`<skills>`+body+`</skills></npc></list>`)
	tpls, err := LoadNPCTemplates(dir, itemTableWithIDs(nil), table, zerolog.Nop())
	if err != nil {
		return nil, err
	}
	tpl, ok := tpls.Get(1)
	if !ok {
		t.Fatal("npc 1 not loaded")
	}
	return tpl, nil
}

func TestLoadNPCTemplateSkillTypes(t *testing.T) {
	t.Parallel()
	a := skill.Ref{ID: 4067, Level: 5}
	aLow := skill.Ref{ID: 4067, Level: 1}
	b := skill.Ref{ID: 4121, Level: 1}
	raceSkill := skill.Ref{ID: npc.RaceSkillID, Level: 2}
	table := skillTableWith(a, aLow, b, raceSkill)

	ok := func(t *testing.T, body string) *npc.Template {
		t.Helper()
		tpl, err := loadNPCSkillsFixture(t, body, table)
		if err != nil {
			t.Fatalf("LoadNPCTemplates: %v", err)
		}
		return tpl
	}
	fails := func(t *testing.T, body, want string) {
		t.Helper()
		_, err := loadNPCSkillsFixture(t, body, table)
		if err == nil {
			t.Fatalf("load of %s succeeded, want an error", body)
		}
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %v, want it to mention %q", err, want)
		}
	}

	t.Run("every listed type holds the skill", func(t *testing.T) {
		got := ok(t, `<skill id="4067" level="5" type="CAPTURE_CANCEL_ALL;DEBUFF1_CANCEL;DEBUFF2_CANCEL"/>`)
		want := map[npc.SkillType]skill.Ref{
			npc.SkillTypeCaptureCancelAll: a,
			npc.SkillTypeDebuff1Cancel:    a,
			npc.SkillTypeDebuff2Cancel:    a,
		}
		if !maps.Equal(got.SkillsByType, want) {
			t.Fatalf("SkillsByType = %v, want %v", got.SkillsByType, want)
		}
	})

	t.Run("PASSIVE token goes to passives only", func(t *testing.T) {
		got := ok(t, `<skill id="4067" level="5" type="PASSIVE;SKILL01_ID"/>`)
		want := map[npc.SkillType]skill.Ref{npc.SkillTypeSkill01ID: a}
		if !maps.Equal(got.SkillsByType, want) {
			t.Fatalf("SkillsByType = %v, want %v", got.SkillsByType, want)
		}
		if len(got.Passives) != 1 || got.Passives[0] != a {
			t.Fatalf("Passives = %v, want [%v]", got.Passives, a)
		}
	})

	t.Run("a later entry with the same type replaces the earlier one", func(t *testing.T) {
		got := ok(t, `<skill id="4067" level="5" type="BUFF"/><skill id="4121" level="1" type="BUFF"/>`)
		if got.SkillsByType[npc.SkillTypeBuff] != b || len(got.SkillsByType) != 1 {
			t.Fatalf("SkillsByType = %v, want only BUFF=%v", got.SkillsByType, b)
		}
		// The replaced skill no longer counts as granted.
		if _, has := got.Skills[4067]; has || got.Skills[4121] != 1 {
			t.Fatalf("Skills = %v, want only 4121=1", got.Skills)
		}
	})

	t.Run("one id under two types: the lower type gives the by-id level", func(t *testing.T) {
		// DD_MAGIC orders before SKILL01_ID, so its level wins even though
		// it is listed first.
		got := ok(t, `<skill id="4067" level="5" type="DD_MAGIC"/><skill id="4067" level="1" type="SKILL01_ID"/>`)
		if got.Skills[4067] != 5 {
			t.Fatalf("Skills[4067] = %d, want 5", got.Skills[4067])
		}
		if got.SkillsByType[npc.SkillTypeDDMagic] != a || got.SkillsByType[npc.SkillTypeSkill01ID] != aLow {
			t.Fatalf("SkillsByType = %v", got.SkillsByType)
		}
	})

	t.Run("a trailing separator is ignored", func(t *testing.T) {
		got := ok(t, `<skill id="4067" level="5" type="BUFF;"/>`)
		if got.SkillsByType[npc.SkillTypeBuff] != a || len(got.SkillsByType) != 1 {
			t.Fatalf("SkillsByType = %v, want only BUFF=%v", got.SkillsByType, a)
		}
	})

	t.Run("a separator-only list grants nothing", func(t *testing.T) {
		got := ok(t, `<skill id="4067" level="5" type=";;"/>`)
		if len(got.SkillsByType) != 0 || len(got.Skills) != 0 || len(got.Passives) != 0 {
			t.Fatalf("SkillsByType = %v Skills = %v Passives = %v, want all empty", got.SkillsByType, got.Skills, got.Passives)
		}
	})

	t.Run("an empty token inside the list fails the load", func(t *testing.T) {
		fails(t, `<skill id="4067" level="5" type="BUFF;;HEAL"/>`, `""`)
	})

	t.Run("an empty type fails the load", func(t *testing.T) {
		fails(t, `<skill id="4067" level="5" type=""/>`, `""`)
	})

	t.Run("an unknown type fails the load", func(t *testing.T) {
		fails(t, `<skill id="4067" level="5" type="BUFF;buff"/>`, `"buff"`)
	})

	t.Run("a missing type fails the load even for an unknown skill", func(t *testing.T) {
		fails(t, `<skill id="99999" level="1"/>`, "type")
	})

	t.Run("an unknown skill is skipped before its types are read", func(t *testing.T) {
		got := ok(t, `<skill id="99999" level="1" type="NOT_A_TYPE"/><skill id="4067" level="5" type="BUFF"/>`)
		if got.SkillsByType[npc.SkillTypeBuff] != a || len(got.SkillsByType) != 1 {
			t.Fatalf("SkillsByType = %v, want only BUFF=%v", got.SkillsByType, a)
		}
	})

	t.Run("the race skill sets the race and grants nothing when no race is set yet", func(t *testing.T) {
		got := ok(t, `<skill id="4416" level="2" type="BUFF"/>`)
		if got.Race != npc.RaceMagicCreature {
			t.Fatalf("Race = %v, want RaceMagicCreature", got.Race)
		}
		if len(got.SkillsByType) != 0 || len(got.Skills) != 0 {
			t.Fatalf("SkillsByType = %v Skills = %v, want both empty", got.SkillsByType, got.Skills)
		}
	})

	t.Run("the race skill after a secondary race marker is an ordinary skill", func(t *testing.T) {
		got := ok(t, `<skill id="4290" level="1" type="PASSIVE"/><skill id="4416" level="2" type="BUFF"/>`)
		if got.Race != npc.RaceUndead {
			t.Fatalf("Race = %v, want RaceUndead", got.Race)
		}
		if got.SkillsByType[npc.SkillTypeBuff] != raceSkill || got.Skills[npc.RaceSkillID] != 2 {
			t.Fatalf("SkillsByType = %v Skills = %v, want BUFF=%v", got.SkillsByType, got.Skills, raceSkill)
		}
	})

	t.Run("no skills block leaves the skill fields nil", func(t *testing.T) {
		dir := t.TempDir()
		writeXMLFixture(t, filepath.Join(dir, "fixture.xml"), `<list><npc id="1" name="x">`+npcRequiredSets+`</npc></list>`)
		tpls, err := LoadNPCTemplates(dir, itemTableWithIDs(nil), table, zerolog.Nop())
		if err != nil {
			t.Fatalf("LoadNPCTemplates: %v", err)
		}
		got, _ := tpls.Get(1)
		if got.SkillsByType != nil || got.Skills != nil || got.Passives != nil {
			t.Fatalf("SkillsByType = %v Skills = %v Passives = %v, want all nil", got.SkillsByType, got.Skills, got.Passives)
		}
	})
}

// TestLoadNPCTemplateSkillTypesDatapack pins the shipped Tyrannosaurus
// (22215): a three-type entry, two ids granted at two levels under two
// types each, and a race skill after a secondary race marker.
func TestLoadNPCTemplateSkillTypesDatapack(t *testing.T) {
	t.Parallel()
	skills, err := LoadSkillDefinitions(datapackPath(t, filepath.Join("data", "xml", "skills")), zerolog.Nop())
	if err != nil {
		t.Fatalf("LoadSkillDefinitions: %v", err)
	}
	dir := t.TempDir()
	src := datapackPath(t, filepath.Join("data", "xml", "npcs", "22000-22999.xml"))
	if err := os.Symlink(src, filepath.Join(dir, "22000-22999.xml")); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	tpls, err := LoadNPCTemplates(dir, itemTableWithIDs(nil), skills, zerolog.Nop())
	if err != nil {
		t.Fatalf("LoadNPCTemplates: %v", err)
	}
	got, ok := tpls.Get(22215)
	if !ok {
		t.Fatal("npc 22215 not loaded")
	}

	ref := func(id, level int) skill.Ref { return skill.Ref{ID: skill.ID(id), Level: level} }
	wantByType := map[npc.SkillType]skill.Ref{
		npc.SkillTypePhysicalSpecial2:  ref(5081, 4),
		npc.SkillTypePhysicalSpecial3:  ref(5082, 4),
		npc.SkillTypePhysicalSpecial1:  ref(5083, 4),
		npc.SkillTypeSelfBuff1:         ref(5087, 1),
		npc.SkillTypeSelfBuff2:         ref(5087, 2),
		npc.SkillTypeDebuff1:           ref(5098, 1),
		npc.SkillTypeDebuff2:           ref(5098, 2),
		npc.SkillTypeCaptureCancelA:    ref(5099, 1),
		npc.SkillTypeCaptureCancelB:    ref(5100, 1),
		npc.SkillTypeCaptureCancelC:    ref(5101, 1),
		npc.SkillTypeCaptureCancelAll:  ref(5102, 1),
		npc.SkillTypeDebuff1Cancel:     ref(5102, 1),
		npc.SkillTypeDebuff2Cancel:     ref(5102, 1),
		npc.SkillTypeLongRangeDDMagic1: ref(5120, 1),
	}
	if !maps.Equal(got.SkillsByType, wantByType) {
		t.Fatalf("SkillsByType = %v, want %v", got.SkillsByType, wantByType)
	}

	wantSkills := map[int]int{5081: 4, 5082: 4, 5083: 4, 5087: 1, 5098: 1, 5099: 1, 5100: 1, 5101: 1, 5102: 1, 5120: 1}
	if !maps.Equal(got.Skills, wantSkills) {
		t.Fatalf("Skills = %v, want %v", got.Skills, wantSkills)
	}

	wantPassives := []skill.Ref{ref(npc.RaceSkillID, 24)}
	if len(got.Passives) != 1 || got.Passives[0] != wantPassives[0] {
		t.Fatalf("Passives = %v, want %v", got.Passives, wantPassives)
	}
	if got.Race != npc.RaceBeast {
		t.Fatalf("Race = %v, want RaceBeast", got.Race)
	}
}
