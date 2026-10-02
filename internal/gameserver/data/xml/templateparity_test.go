package xml

import (
	"path/filepath"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/rs/zerolog"
)

// The expectations in this file come from loading the same fixtures with the
// reference loaders (DocumentSkill / DocumentItem built from
// aCis_gameserver/java, OpenJDK 21) and dumping the loaded skills' and
// items' func and effect templates. A skill fixture the reference failed to
// load shows here as a skill with no levels: the reference drops the whole
// file, Go drops each failing level (#3088 tracks that granularity).

// loadTemplateParitySkill loads a two-level skill with one enchant level whose #t
// table holds 1 and 2 and whose #st table holds a and b.
func loadTemplateParitySkill(t *testing.T, body string) *skill.Table {
	t.Helper()
	dir := t.TempDir()
	writeXMLFixture(t, filepath.Join(dir, "fixture.xml"),
		`<list><skill id="1" name="x" levels="2" enchantLevels1="1">`+
			`<table name="#t"> 1 2 </table><table name="#st"> a b </table>`+
			`<set name="target" val="SELF"/><set name="skillType" val="BUFF"/><set name="operateType" val="ACTIVE"/>`+
			body+`</skill></list>`)
	table, err := LoadSkillDefinitions(dir, zerolog.Nop())
	if err != nil {
		t.Fatalf("LoadSkillDefinitions: %v", err)
	}
	return table
}

var templateParityLevels = []int{1, 2, 101}

// TestEffectStackTypeIsReadAsWritten: an effect's stackType never reads the
// skill's tables (probe: every level's stack type is "#st").
func TestEffectStackTypeIsReadAsWritten(t *testing.T) {
	t.Parallel()
	table := loadTemplateParitySkill(t, `<for><effect name="Buff" val="0" time="1" stackType="#st"/></for>`)
	for _, level := range templateParityLevels {
		def, ok := table.Get(1, level)
		if !ok {
			t.Fatalf("level %d not loaded", level)
		}
		if got := def.Effects[0].StackType; got != "#st" {
			t.Errorf("level %d stack type = %q, want #st", level, got)
		}
	}
}

// TestShippedFrintezzaSongsStackTypeIsLiteral: skill 5008's first effect
// names its stack type through a table the reference never reads for it, so
// every level shares the literal stack type "#stackType".
func TestShippedFrintezzaSongsStackTypeIsLiteral(t *testing.T) {
	t.Parallel()
	skills, err := LoadSkillDefinitions(datapackPath(t, filepath.Join("data", "xml", "skills")), zerolog.Nop())
	if err != nil {
		t.Fatalf("LoadSkillDefinitions: %v", err)
	}
	for level := 1; level <= 5; level++ {
		def, ok := skills.Get(5008, level)
		if !ok {
			t.Fatalf("skill 5008 level %d not loaded", level)
		}
		if got := def.Effects[0].StackType; got != "#stackType" {
			t.Errorf("skill 5008 level %d stack type = %q, want #stackType", level, got)
		}
		if got := def.Effects[1].StackType; got != "none" {
			t.Errorf("skill 5008 level %d second effect stack type = %q, want none", level, got)
		}
	}
}

// TestSkillTemplateGrammarRejections: each of these fails the reference's
// load, so no level of the skill loads.
func TestSkillTemplateGrammarRejections(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"for func stat table ref":    `<for><add stat="#t" val="1"/></for>`,
		"effect func stat table ref": `<for><effect name="Buff" val="0" time="1"><add stat="#t" val="1"/></effect></for>`,
		"unknown func stat":          `<for><add stat="bogus" val="1"/></for>`,
		"nested effect":              `<for><effect name="Buff" val="0" time="1"><effect name="Buff" val="0"/></effect></for>`,
		"unknown effectType":         `<for><effect name="Buff" val="0" time="1" effectType="BOGUS"/></for>`,
		"unknown chanceType":         `<for><effect name="Buff" val="0" time="1" chanceType="BOGUS"/></for>`,
		"empty chanceType":           `<for><effect name="Buff" val="0" time="1" chanceType=""/></for>`,
		"empty effect name":          `<for><effect name="" val="0" time="1"/></for>`,
	}
	for name, body := range cases {
		table := loadTemplateParitySkill(t, body)
		for _, level := range templateParityLevels {
			if _, ok := table.Get(1, level); ok {
				t.Errorf("%s: level %d loaded, want rejected", name, level)
			}
		}
	}
}

// TestSkillEffectGrammarAccepts: a known effectType loads (probe: type STUN).
func TestSkillEffectGrammarAccepts(t *testing.T) {
	t.Parallel()
	def, ok := loadTemplateParitySkill(t, `<for><effect name="Buff" val="0" time="1" effectType="STUN" chanceType="ON_HIT"/></for>`).Get(1, 1)
	if !ok {
		t.Fatal("skill not loaded")
	}
	if eff := def.Effects[0]; eff.EffectType != "STUN" || eff.ChanceType != "ON_HIT" {
		t.Errorf("effect = %+v, want effect type STUN and chance type ON_HIT", eff)
	}
}

// TestEffectSkipsUnknownChildTag: an <effect> child that is neither a func
// nor an <effect> is ignored (probe: the effect keeps its one pAtk func).
func TestEffectSkipsUnknownChildTag(t *testing.T) {
	t.Parallel()
	table := loadTemplateParitySkill(t, `<for><effect name="Buff" val="0" time="1"><note/><add stat="pAtk" val="1"/></effect></for>`)
	for _, level := range templateParityLevels {
		def, ok := table.Get(1, level)
		if !ok {
			t.Fatalf("level %d not loaded", level)
		}
		if funcs := def.Effects[0].Funcs; len(funcs) != 1 || funcs[0].Stat != "pAtk" {
			t.Errorf("level %d effect funcs = %+v, want one pAtk func", level, funcs)
		}
	}
}

// TestTemplateCondNeedsToBeFirstNode: a <for>, <enchant1for> or <effect>
// block reads its <cond> only when nothing, not even whitespace, precedes
// it; comments do not count. An unread <cond> is not parsed, so its "#t"
// msgId does not reject the skill.
func TestTemplateCondNeedsToBeFirstNode(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		body     string
		attached bool
	}{
		{"for, no leading node", `<for><cond msg="m"><player level="1"/></cond><add stat="pAtk" val="1"/></for>`, true},
		{"for, leading comment", `<for><!-- c --><cond msg="m"><player level="1"/></cond><add stat="pAtk" val="1"/></for>`, true},
		{"for, leading whitespace", "<for>\n  <cond msgId=\"#t\"><player level=\"1\"/></cond><add stat=\"pAtk\" val=\"1\"/></for>", false},
		{"effect, no leading node", `<for><effect name="Buff" val="0" time="1"><cond msg="m"><player level="1"/></cond><add stat="pAtk" val="1"/></effect></for>`, true},
		{"effect, leading whitespace", "<for><effect name=\"Buff\" val=\"0\" time=\"1\">\n <cond msgId=\"#t\"><player level=\"1\"/></cond><add stat=\"pAtk\" val=\"1\"/></effect></for>", false},
	}
	for _, tc := range cases {
		table := loadTemplateParitySkill(t, tc.body)
		for _, level := range templateParityLevels {
			def, ok := table.Get(1, level)
			if !ok {
				t.Errorf("%s: level %d not loaded", tc.name, level)
				continue
			}
			funcs := def.Funcs
			if len(def.Effects) == 1 {
				funcs = def.Effects[0].Funcs
			}
			if len(funcs) != 1 {
				t.Fatalf("%s: level %d has %d funcs, want 1", tc.name, level, len(funcs))
			}
			if got := funcs[0].AttachCondition != nil; got != tc.attached {
				t.Errorf("%s: level %d func gated = %v, want %v", tc.name, level, got, tc.attached)
			}
		}
	}

	// <enchant1for> follows the same rule (probe: level 101 holds one
	// ungated pDef func).
	def, ok := loadTemplateParitySkill(t, "<for><add stat=\"pAtk\" val=\"1\"/></for><enchant1for>\n<cond msgId=\"#t\"><player level=\"1\"/></cond><add stat=\"pDef\" val=\"1\"/></enchant1for>").Get(1, 101)
	if !ok {
		t.Fatal("enchant1for: level 101 not loaded")
	}
	if len(def.Funcs) != 1 || def.Funcs[0].Stat != "pDef" || def.Funcs[0].AttachCondition != nil {
		t.Errorf("enchant1for: level 101 funcs = %+v, want one ungated pDef func", def.Funcs)
	}
}

// TestItemEffectReadWithSkillGrammar: an item's <for><effect> is read with
// the skill effect grammar and discarded; any failure skips the item. Item
// 52 shows that a ChanceSkillTrigger effect needs neither triggeredId nor
// chanceType at load.
func TestItemEffectReadWithSkillGrammar(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeItemFile(t, dir, "fixture.xml", `
		<item id="40" type="EtcItem" name="a"><for><effect name="Buff" val="0"><effect name="Buff" val="0"/></effect></for></item>
		<item id="41" type="EtcItem" name="a"><for><effect name="Buff" val="0"><add stat="bogus" val="1"/></effect></for></item>
		<item id="42" type="EtcItem" name="a"><for><effect name="Buff" val="0"><add stat="pAtk" val="x"/></effect></for></item>
		<item id="43" type="EtcItem" name="a"><for><effect name="Buff" val="0" count="x"/></for></item>
		<item id="44" type="EtcItem" name="a"><for><effect name="Buff" val="0" time="1.5"/></for></item>
		<item id="45" type="EtcItem" name="a"><for><effect name="Buff" val="0" self="x"/></for></item>
		<item id="46" type="EtcItem" name="a"><for><effect name="Buff" val="0" noicon="x"/></for></item>
		<item id="47" type="EtcItem" name="a"><for><effect name="Buff" val="0" stackOrder="x"/></for></item>
		<item id="48" type="EtcItem" name="a"><for><effect name="Buff" val="0" effectPower="x"/></for></item>
		<item id="49" type="EtcItem" name="a"><for><effect name="Buff" val="0" effectType="BOGUS"/></for></item>
		<item id="50" type="EtcItem" name="a"><for><effect name="Buff" val="0" abnormal="bogus"/></for></item>
		<item id="51" type="EtcItem" name="a"><for><effect name="Buff" val="0" chanceType="BOGUS"/></for></item>
		<item id="52" type="EtcItem" name="a"><for><effect name="ChanceSkillTrigger" val="0"/></for></item>
		<item id="53" type="EtcItem" name="a"><table name="#t"> 5 </table><for><effect name="Buff" val="#t" count="2" time="10" self="1" noicon="1" stackType="x" stackOrder="1.5" effectPower="20" effectType="STUN" triggeredId="1" triggeredLevel="2" chanceType="ON_HIT" activationChance="10" abnormal="stun"><cond msg="m"><player level="1"/></cond><add stat="pAtk" val="#t"><player level="1"/></add><mul stat="pDef" val="2"/></effect></for></item>
		<item id="55" type="EtcItem" name="a"><for><effect name="Buff" val="0"><note/><add stat="pAtk" val="1"/></effect></for></item>
		<item id="56" type="EtcItem" name="a"><for><add stat="bogus" val="1"/></for></item>
		<item id="60" type="EtcItem" name="a"><for><effect name="Buff" val="0">
			<cond msgId="#10"><player level="1"/></cond><add stat="pAtk" val="1"/></effect></for></item>
		<item id="61" type="EtcItem" name="a"><for><effect name="Buff" val="0" triggeredId="x"/></for></item>
		<item id="62" type="EtcItem" name="a"><for><effect name="Buff" val="0" activationChance="x"/></for></item>
		<item id="63" type="EtcItem" name="a"><for><effect name="" val="0"/></for></item>
		<item id="64" type="EtcItem" name="a"><for><effect name="Buff"/></for></item>
		<item id="65" type="EtcItem" name="a"><for><effect name="Buff" val="0" chanceType=""/></for></item>
		<item id="66" type="EtcItem" name="a"><for><effect name="Buff" val="0" triggeredLevel="x"/></for></item>
		<item id="67" type="EtcItem" name="a"><for><effect name="Buff" val="0"><add stat="#t" val="1"/></effect></for></item>
		<item id="68" type="EtcItem" name="a"><for><add stat="#t" val="1"/></for></item>
		<item id="99" type="EtcItem" name="a"></item>`)

	table, err := LoadItemTemplates(dir, zerolog.Nop())
	if err != nil {
		t.Fatalf("LoadItemTemplates: %v", err)
	}
	loaded := map[int32]bool{52: true, 53: true, 55: true, 60: true, 99: true}
	for _, id := range []int32{40, 41, 42, 43, 44, 45, 46, 47, 48, 49, 50, 51, 52, 53, 55, 56, 60, 61, 62, 63, 64, 65, 66, 67, 68, 99} {
		tpl, ok := table.Get(id)
		if ok != loaded[id] {
			t.Errorf("item %d loaded = %v, want %v", id, ok, loaded[id])
			continue
		}
		if ok && len(tpl.Modifiers) != 0 {
			t.Errorf("item %d modifiers = %+v, want none (an item effect is discarded)", id, tpl.Modifiers)
		}
	}
}

// TestItemForCondNeedsToBeFirstNode: an item <for> reads its <cond> only
// when it is the block's first node and holds a predicate; an unread one is
// not parsed (probe: 57 and 59 ungated, 58 gated).
func TestItemForCondNeedsToBeFirstNode(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeItemFile(t, dir, "fixture.xml", `
		<item id="57" type="EtcItem" name="a"><for>
			<cond msgId="#10"><player level="1"/></cond><add stat="pAtk" val="1"/></for></item>
		<item id="58" type="EtcItem" name="a"><for><cond msg="x"><player level="1"/></cond><add stat="pAtk" val="1"/></for></item>
		<item id="59" type="EtcItem" name="a"><for><cond msgId="1"></cond><add stat="pAtk" val="1"/></for></item>`)

	table, err := LoadItemTemplates(dir, zerolog.Nop())
	if err != nil {
		t.Fatalf("LoadItemTemplates: %v", err)
	}
	for id, gated := range map[int32]bool{57: false, 58: true, 59: false} {
		tpl, ok := table.Get(id)
		if !ok {
			t.Errorf("item %d not loaded", id)
			continue
		}
		if len(tpl.Modifiers) != 1 || tpl.Modifiers[0].Stat != "pAtk" {
			t.Errorf("item %d modifiers = %+v, want one pAtk func", id, tpl.Modifiers)
			continue
		}
		if got := tpl.Modifiers[0].AttachCondition != nil; got != gated {
			t.Errorf("item %d func gated = %v, want %v", id, got, gated)
		}
	}
}
