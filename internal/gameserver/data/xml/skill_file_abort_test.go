package xml

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/rs/zerolog"
)

// abortSets are the <set> values every fixture skill needs to build.
const abortSets = `<set name="target" val="ONE"/><set name="skillType" val="PDAM"/><set name="operateType" val="ACTIVE"/>`

// abortFile is a skill file holding a valid skill 1, the skill element open
// (default: skill 2 with one level) with the standard sets plus body, and a
// valid skill 3.
func abortFile(open, body string) string {
	if open == "" {
		open = `<skill id="2" name="b" levels="1">`
	}
	return `<list>` +
		`<skill id="1" name="a" levels="1">` + abortSets + `</skill>` +
		open + abortSets + body + `</skill>` +
		`<skill id="3" name="c" levels="1">` + abortSets + `</skill>` +
		`</list>`
}

// TestSkillFileStopsAtMalformedSkill covers #3088, #3265 and #3270's file
// granularity. DocumentSkill.parseDocument (DocumentSkill.java:84-108) does
// not catch what parseSkill throws, so DocumentBase.parse
// (DocumentBase.java:81-101) logs "Error loading file" and stops: the failing
// skill and every later one in its file are not loaded, the earlier ones are.
// Each case puts the failure in skill 2 between a valid skill 1 and a valid
// skill 3.
func TestSkillFileStopsAtMalformedSkill(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, open, body string
	}{
		// parseSkill: Integer.parseInt and array sizes.
		{name: "malformed id", open: `<skill id="2x" name="b" levels="1">`},
		{name: "malformed levels", open: `<skill id="2" name="b" levels="one">`},
		{name: "negative levels", open: `<skill id="2" name="b" levels="-1">`},
		{name: "levels beyond int32", open: `<skill id="2" name="b" levels="4294967296">`},
		{name: "malformed enchantLevels1", open: `<skill id="2" name="b" levels="1" enchantLevels1="x">`},
		{name: "empty enchantLevels2", open: `<skill id="2" name="b" levels="1" enchantLevels2="">`},
		{name: "negative enchantLevels1", open: `<skill id="2" name="b" levels="1" enchantLevels1="-2">`},
		{name: "missing name", open: `<skill id="2" levels="1">`},
		// parseTable.
		{name: "table name without '#'", body: `<table name="mp">1</table>`},
		// parse*Condition: values decoded while the condition loads.
		{name: "malformed condition integer", body: `<cond><player level="oops"/></cond>`},
		{name: "malformed msgId", body: `<cond msgId="oops"><player level="1"/></cond>`},
		{name: "one-part hp_min_max", body: `<cond><target hp_min_max="30"/></cond>`},
		{name: "malformed clanHall entry", body: `<cond><player clanHall="21, x"/></cond>`},
		{name: "battle_force beyond a byte", body: `<cond><player battle_force="200"/></cond>`},
		{name: "unknown race", body: `<cond><player race="GIANT"/></cond>`},
		{name: "unknown skill condition stat", body: `<cond><skill stat="bogus"/></cond>`},
		{name: "skill condition without stat", body: `<cond><skill/></cond>`},
		{name: "insidePoly without zone", body: `<cond><player insidePoly="true"/></cond>`},
		{name: "insidePoly zone without maxZ", body: `<cond><player insidePoly="true"><zone minZ="0"><node x="0" y="0"/></zone></player></cond>`},
		{name: "malformed condition integer in a for block", body: `<for><cond><player level="oops"/></cond><add stat="pAtk" val="1"/></for>`},
		{name: "malformed condition integer in a func", body: `<for><add stat="pAtk" val="1"><player level="oops"/></add></for>`},
		{name: "unresolved table read as an empty integer", body: `<cond><player charges="#missing"/></cond>`},
		{name: "table reference where no table can be read", body: `<for><effect name="Buff" val="0"><cond><player level="#lvl"/></cond></effect></for>`},
		// attachEffect / attachFunc.
		{name: "malformed effect count", body: `<for><effect name="Buff" val="0" count="oops"/></for>`},
		{name: "malformed func value", body: `<for><add stat="pAtk" val="x"/></for>`},
		{name: "unknown effect name", body: `<for><effect name="Bogus" val="0"/></for>`},
		{name: "effect name of a class without an effect constructor", body: `<for><effect name="Template" val="0"/></for>`},
		{name: "fully-qualified effect name", body: `<for><effect name="net.sf.l2j.gameserver.skills.effects.EffectChanceSkillTrigger" val="0" triggeredId="1" chanceType="ON_HIT"/></for>`},
		{name: "nested effect", body: `<for><effect name="Buff" val="0"><effect name="Buff" val="0"/></effect></for>`},
		// makeSkills drops a level, then a template reads its position.
		{name: "level that does not build, with a for block", body: `<set name="mpConsume" val="x"/><for><add stat="pAtk" val="1"/></for>`},
		{name: "unknown skillType, with a cond", body: `<set name="skillType" val="BOGUS"/><cond><player level="1"/></cond>`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			writeXMLFixture(t, filepath.Join(dir, "fixture.xml"), abortFile(c.open, c.body))

			var buf bytes.Buffer
			table, err := LoadSkillDefinitions(dir, zerolog.New(&buf))
			if err != nil {
				t.Fatalf("LoadSkillDefinitions: %v", err)
			}
			if _, ok := table.Get(1, 1); !ok {
				t.Fatal("skill 1, before the malformed skill, not loaded")
			}
			for _, id := range []skill.ID{2, 3} {
				if _, ok := table.Get(id, 1); ok {
					t.Fatalf("skill %d loaded; the file stops at skill 2", id)
				}
			}
			if !strings.Contains(buf.String(), "fixture.xml") {
				t.Fatalf("log = %q, want it to name the file", buf.String())
			}
		})
	}
}

// TestSkillFileStopDoesNotReachOtherFiles checks that a file stopped at a
// malformed skill leaves the other files loading.
func TestSkillFileStopDoesNotReachOtherFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeXMLFixture(t, filepath.Join(dir, "a.xml"), abortFile("", `<cond><player level="oops"/></cond>`))
	writeXMLFixture(t, filepath.Join(dir, "b.xml"), `<list><skill id="4" name="d" levels="1">`+abortSets+`</skill></list>`)

	table, err := LoadSkillDefinitions(dir, zerolog.Nop())
	if err != nil {
		t.Fatalf("LoadSkillDefinitions: %v", err)
	}
	for id, want := range map[skill.ID]bool{1: true, 2: false, 3: false, 4: true} {
		if _, ok := table.Get(id, 1); ok != want {
			t.Fatalf("skill %d loaded = %v, want %v", id, ok, want)
		}
	}
}

// TestDroppedLevelShiftsLaterTemplates checks that templates attach by
// position among the built levels, as DocumentSkill.parseSkill indexes
// currentSkills (DocumentSkill.java:205-320). Level 2's unknown skillType
// drops it, so the built levels are 1, 101, 141, 142, and the enchant1 route's
// template, read for position 2, lands on level 141.
func TestDroppedLevelShiftsLaterTemplates(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeXMLFixture(t, filepath.Join(dir, "fixture.xml"), `<list>
		<skill id="7" name="x" levels="2" enchantLevels1="1" enchantLevels2="2">
		<table name="#type"> PDAM BOGUS </table>
		<set name="target" val="ONE"/><set name="skillType" val="#type"/><set name="operateType" val="ACTIVE"/>
		<enchant1 name="skillType" val="PDAM"/>
		<enchant2 name="skillType" val="PDAM"/>
		<enchant1for><add stat="pAtk" val="7"/></enchant1for>
	</skill></list>`)

	table, err := LoadSkillDefinitions(dir, zerolog.Nop())
	if err != nil {
		t.Fatalf("LoadSkillDefinitions: %v", err)
	}
	if _, ok := table.Get(7, 2); ok {
		t.Fatal("level 2 loaded; its skillType is unknown")
	}
	for _, level := range []int{1, 101, 141, 142} {
		def, ok := table.Get(7, level)
		if !ok {
			t.Fatalf("level %d not loaded", level)
		}
		wantFuncs := 0
		if level == 141 {
			wantFuncs = 1
		}
		if len(def.Funcs) != wantFuncs {
			t.Fatalf("level %d funcs = %+v, want %d", level, def.Funcs, wantFuncs)
		}
	}
}

// TestUnknownSkillTypeDropsItsRoute covers #3270: a level's skillType must
// name a SkillType exactly (StatSet.getEnum -> Enum.valueOf in
// DocumentSkill.makeSkills), on every route. The failing level drops with the
// later levels of its route; other routes load.
func TestUnknownSkillTypeDropsItsRoute(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		body    string
		dropped []int
		loaded  []int
	}{
		{name: "regular", body: `<table name="#type"> PDAM pdam PDAM </table><set name="skillType" val="#type"/>`, dropped: []int{2, 3}, loaded: []int{1, 101, 102, 141}},
		{name: "enchant1", body: `<enchant1 name="skillType" val="BOGUS"/>`, dropped: []int{101, 102}, loaded: []int{1, 2, 3, 141}},
		{name: "enchant2", body: `<enchant2 name="skillType" val="Pdam"/>`, dropped: []int{141}, loaded: []int{1, 2, 3, 101, 102}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			writeXMLFixture(t, filepath.Join(dir, "fixture.xml"), `<list><skill id="8" name="x" levels="3" enchantLevels1="2" enchantLevels2="1">`+
				abortSets+c.body+`</skill></list>`)
			table, err := LoadSkillDefinitions(dir, zerolog.Nop())
			if err != nil {
				t.Fatalf("LoadSkillDefinitions: %v", err)
			}
			for _, level := range c.dropped {
				if _, ok := table.Get(8, level); ok {
					t.Fatalf("level %d loaded, want dropped", level)
				}
			}
			for _, level := range c.loaded {
				if def, ok := table.Get(8, level); !ok || def.SkillType != "PDAM" {
					t.Fatalf("level %d = %v (skillType %q), want loaded as PDAM", level, ok, def.SkillType)
				}
			}
		})
	}
}

// TestSkillFuncTagsMatchIgnoringCase covers #3271: DocumentBase.parseTemplate
// (DocumentBase.java:145-174) matches every func tag with equalsIgnoreCase,
// under a skill's <for> and inside its <effect>.
func TestSkillFuncTagsMatchIgnoringCase(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeXMLFixture(t, filepath.Join(dir, "fixture.xml"), skillFixture(`<for>`+
		`<Add stat="pAtk" val="1"/><SubDiv stat="pDef" val="2"/><BaseMUL stat="mAtk" val="3"/>`+
		`<effect name="Buff" val="0"><aDD stat="runSpd" val="4"/><EnChAnT stat="mDef" val="5"/></effect>`+
		`</for>`))

	table, err := LoadSkillDefinitions(dir, zerolog.Nop())
	if err != nil {
		t.Fatalf("LoadSkillDefinitions: %v", err)
	}
	def, ok := table.Get(1, 1)
	if !ok {
		t.Fatal("skill 1 not loaded")
	}
	want := []skill.FuncTemplate{
		{Op: skill.FuncAdd, Stat: "pAtk", Value: 1},
		{Op: skill.FuncSubDiv, Stat: "pDef", Value: 2},
		{Op: skill.FuncBaseMul, Stat: "mAtk", Value: 3},
	}
	if len(def.Funcs) != len(want) {
		t.Fatalf("funcs = %+v, want %+v", def.Funcs, want)
	}
	for i, fn := range def.Funcs {
		if fn.Op != want[i].Op || fn.Stat != want[i].Stat || fn.Value != want[i].Value {
			t.Fatalf("func %d = %+v, want %+v", i, fn, want[i])
		}
	}
	if len(def.Effects) != 1 {
		t.Fatalf("effects = %+v, want one", def.Effects)
	}
	effFuncs := def.Effects[0].Funcs
	if len(effFuncs) != 2 || effFuncs[0].Op != skill.FuncAdd || effFuncs[0].Stat != "runSpd" ||
		effFuncs[1].Op != skill.FuncEnchant || effFuncs[1].Stat != "mDef" {
		t.Fatalf("effect funcs = %+v, want add runSpd then enchant mDef", effFuncs)
	}
}
