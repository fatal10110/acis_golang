package xml

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/rs/zerolog"
)

// The expectations in this file come from loading the same fixtures with
// the reference loaders (DocumentSkill / DocumentItem from the aCis build
// classes, OpenJDK 21.0.11) and dumping the parsed conditions. A table
// reference read without a skill as the template (a null template, an item,
// or an <effect>) threw IllegalStateException there and the skill or item
// did not load; one read with the skill as the template resolved against
// the level's table row; one read as written stayed "#t".

// condTableSkill is a two-level skill whose #t table holds 1 and 2.
func condTableSkill(body string) string {
	return `<list><skill id="1" name="x" levels="2" enchantLevels1="1">` +
		`<table name="#t"> 1 2 </table>` +
		`<set name="target" val="SELF"/><set name="skillType" val="BUFF"/><set name="operateType" val="ACTIVE"/>` +
		body + `</skill></list>`
}

func loadCondTableSkill(t *testing.T, body string) *skill.Table {
	t.Helper()
	dir := t.TempDir()
	writeXMLFixture(t, filepath.Join(dir, "fixture.xml"), condTableSkill(body))
	table, err := LoadSkillDefinitions(dir, zerolog.Nop())
	if err != nil {
		t.Fatalf("LoadSkillDefinitions: %v", err)
	}
	return table
}

func TestSkillConditionTableRefsRejectedWithoutSkillTemplate(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"cond msgId":                  `<cond msgId="#t"><player level="1"/></cond>`,
		"for cond msgId":              `<for><cond msgId="#t"><player level="1"/></cond><add stat="pAtk" val="1"/></for>`,
		"player hp":                   `<cond><player hp="#t"/></cond>`,
		"player hp under and/not":     `<cond><and><player level="1"/><not><player mp="#t"/></not></and></cond>`,
		"player clanHall entry":       `<cond><player clanHall="#t"/></cond>`,
		"player insidePoly zone minZ": `<cond><player insidePoly="true"><zone minZ="#t" maxZ="10"><node x="0" y="0"/><node x="10" y="0"/><node x="10" y="10"/></zone></player></cond>`,
		"player insidePoly node x":    `<cond><player insidePoly="true"><zone minZ="0" maxZ="10"><node x="#t" y="0"/><node x="10" y="0"/><node x="10" y="10"/></zone></player></cond>`,
		"target npcId trimmed entry":  `<cond><target npcId="1, #t"/></cond>`,
		"target race_id":              `<cond><target race_id="#t"/></cond>`,
		"effect cond level":           `<for><effect name="Buff" val="0" time="1"><cond><player level="#t"/></cond><add stat="pAtk" val="1"/></effect></for>`,
		"effect func cond level":      `<for><effect name="Buff" val="0" time="1"><add stat="pAtk" val="1"><player level="#t"/></add></effect></for>`,
		"effect cond pair part":       `<for><effect name="Buff" val="0" time="1"><cond><player active_skill_id_lvl="7,#t"/></cond><add stat="pAtk" val="1"/></effect></for>`,
		"not child":                   `<cond><not><player hp="#t"/></not></cond>`,
	}
	for name, body := range cases {
		table := loadCondTableSkill(t, body)
		for _, level := range []int{1, 2, 101} {
			if _, ok := table.Get(1, level); ok {
				t.Errorf("%s: level %d loaded, want it rejected", name, level)
			}
		}
	}
}

func TestSkillConditionTableRefsResolveWithSkillTemplate(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		body string
		attr func(def skill.Definition) map[string]string
		key  string
		want [3]string // levels 1, 2 and 101
	}{
		{
			name: "player level", body: `<cond><player level="#t"/></cond>`,
			attr: rootAttrs, key: "level", want: [3]string{"1", "2", "2"},
		},
		{
			name: "player Charges", body: `<cond><player Charges="#t"/></cond>`,
			attr: rootAttrs, key: "Charges", want: [3]string{"1", "2", "2"},
		},
		{
			name: "player pair second part", body: `<cond><player active_skill_id_lvl="7,#t"/></cond>`,
			attr: rootAttrs, key: "active_skill_id_lvl", want: [3]string{"7,1", "7,2", "7,2"},
		},
		{
			name: "target pair second part", body: `<cond><target hp_min_max="10,#t"/></cond>`,
			attr: rootAttrs, key: "hp_min_max", want: [3]string{"10,1", "10,2", "10,2"},
		},
		{
			name: "func cond level", body: `<for><add stat="pAtk" val="1"><player level="#t"/></add></for>`,
			attr: func(def skill.Definition) map[string]string { return def.Funcs[0].Condition.Attrs },
			key:  "level", want: [3]string{"1", "2", "2"},
		},
		{
			name: "boolean read as written", body: `<cond><player resting="#t"/></cond>`,
			attr: rootAttrs, key: "resting", want: [3]string{"#t", "#t", "#t"},
		},
		{
			// <not> reads only its first child; the second is never parsed.
			name: "not reads its first child only", body: `<cond><not><player level="#t"/><player hp="#t"/></not></cond>`,
			attr: func(def skill.Definition) map[string]string { return def.Conditions[0].Root.Children[1].Attrs },
			key:  "hp", want: [3]string{"#t", "#t", "#t"},
		},
	}
	for _, tc := range cases {
		table := loadCondTableSkill(t, tc.body)
		for i, level := range []int{1, 2, 101} {
			def, ok := table.Get(1, level)
			if !ok {
				t.Errorf("%s: level %d not loaded", tc.name, level)
				continue
			}
			if got := tc.attr(def)[tc.key]; got != tc.want[i] {
				t.Errorf("%s: level %d %s = %q, want %q", tc.name, level, tc.key, got, tc.want[i])
			}
		}
	}
}

func rootAttrs(def skill.Definition) map[string]string { return def.Conditions[0].Root.Attrs }

func TestSkillConditionMessagesFollowReference(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		body    string
		level   int
		message string
		id      int32
	}{
		{"msg read as written", `<cond msg="#t"><player level="1"/></cond>`, 1, "#t", 0},
		{"msg shadows a table msgId", `<cond msg="x" msgId="#t"><player level="1"/></cond>`, 1, "x", 0},
		{"signed hash literal msgId", `<cond msgId="-#10"><player level="1"/></cond>`, 1, "", -16},
		{"enchant cond never reads msgId", `<enchant1cond msgId="#t"><player level="1"/></enchant1cond>`, 101, "", 0},
	}
	for _, tc := range cases {
		table := loadCondTableSkill(t, tc.body)
		def, ok := table.Get(1, tc.level)
		if !ok {
			t.Errorf("%s: level %d not loaded", tc.name, tc.level)
			continue
		}
		c := def.Conditions[0]
		if c.Message != tc.message || c.MessageID != tc.id {
			t.Errorf("%s: message %q id %d, want %q id %d", tc.name, c.Message, c.MessageID, tc.message, tc.id)
		}
	}
}

func TestItemConditionTableRefsFollowReference(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeItemFile(t, dir, "fixture.xml", `
		<item id="1" type="EtcItem" name="a"><table name="#t"> 5 </table><cond msgId="#10"><player level="1"/></cond></item>
		<item id="2" type="EtcItem" name="a"><table name="#t"> 5 </table><cond><player level="#t"/></cond></item>
		<item id="3" type="EtcItem" name="a"><cond><player resting="#t"/></cond></item>
		<item id="4" type="EtcItem" name="a"><cond msg="#t"><player level="1"/></cond></item>
		<item id="5" type="EtcItem" name="a"><for><add stat="pAtk" val="1"><player hp="#t"/></add></for></item>
		<item id="6" type="EtcItem" name="a"><for><effect name="#t" val="0"/></for></item>
		<item id="7" type="EtcItem" name="a"><table name="#t"> 5 </table><for><effect name="Buff" val="#t"/></for></item>
		<item id="8" type="EtcItem" name="a"><cond><target npcId="1, #2"/></cond></item>
		<item id="9" type="EtcItem" name="a"><cond msgId="-#10"><player level="1"/></cond></item>
		<item id="10" type="EtcItem" name="a"><cond msg="x" msgId="#10"><player level="1"/></cond></item>
		<item id="12" type="EtcItem" name="a"><for><effect name="Buff" val="0" count="#1"/></for></item>
		<item id="13" type="EtcItem" name="a"><for><effect name="Buff" val="0" stackType="#1"/></for></item>
		<item id="20" type="EtcItem" name="a"><for><effect name="Buff" val="0"><cond msgId="#10"><player level="1"/></cond></effect></for></item>
		<item id="21" type="EtcItem" name="a"><for><effect name="Buff" val="0"><cond><player level="#1"/></cond></effect></for></item>
		<item id="22" type="EtcItem" name="a"><for><effect name="Buff" val="0"><add stat="pAtk" val="1"><player level="#1"/></add></effect></for></item>
		<item id="23" type="EtcItem" name="a"><table name="#t"> 5 </table><for><effect name="Buff" val="0"><add stat="pAtk" val="#t"/></effect></for></item>
		<item id="24" type="EtcItem" name="a"><cond><not><player level="1"/><player hp="#t"/></not></cond></item>
		<item id="25" type="EtcItem" name="a"><cond><not><player hp="#t"/></not></cond></item>
		<item id="26" type="EtcItem" name="a"><for><effect name="Buff" val="0"><cond msg="x" msgId="#10"><player level="1"/></cond></effect></for></item>
		<item id="30" type="EtcItem" name="a"><for><effect name="Buff" val="0"><add stat="pAtk" val="1"/><cond msgId="#10"><player level="#1"/></cond></effect></for></item>
		<item id="99" type="EtcItem" name="a"></item>`)

	table, err := LoadItemTemplates(dir, zerolog.Nop())
	if err != nil {
		t.Fatalf("LoadItemTemplates: %v", err)
	}
	loaded := map[int32]bool{3: true, 4: true, 7: true, 9: true, 10: true, 13: true, 23: true, 24: true, 26: true, 30: true, 99: true}
	for _, id := range []int32{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 12, 13, 20, 21, 22, 23, 24, 25, 26, 30, 99} {
		tpl, ok := table.Get(id)
		if ok != loaded[id] {
			t.Errorf("item %d loaded = %v, want %v", id, ok, loaded[id])
			continue
		}
		if !ok {
			continue
		}
		var got string
		switch id {
		case 3:
			got = tpl.UseConditions[0].Root.Attrs["resting"]
		case 4, 10:
			got = tpl.UseConditions[0].Message
		case 9:
			got = fmt.Sprint(tpl.UseConditions[0].MessageID)
		default:
			continue
		}
		want := map[int32]string{3: "#t", 4: "#t", 9: "-16", 10: "x"}[id]
		if got != want {
			t.Errorf("item %d read %q, want %q", id, got, want)
		}
	}
}

// TestShippedConditionTableRefsLoadAsBefore: every table reference the
// shipped skills and items put in a condition sits where the reference
// resolves it, so nothing shipped is rejected and every one still resolves,
// as it did when this loader resolved table references everywhere. Skill
// 2274 level 3 anchors the scan on a real resolved pair.
func TestShippedConditionTableRefsLoadAsBefore(t *testing.T) {
	t.Parallel()
	var skillLog bytes.Buffer
	skills, err := LoadSkillDefinitions(datapackPath(t, filepath.Join("data", "xml", "skills")), zerolog.New(&skillLog))
	if err != nil {
		t.Fatalf("LoadSkillDefinitions: %v", err)
	}
	var itemLog bytes.Buffer
	items, err := LoadItemTemplates(datapackPath(t, filepath.Join("data", "xml", "items")), zerolog.New(&itemLog))
	if err != nil {
		t.Fatalf("LoadItemTemplates: %v", err)
	}
	for name, log := range map[string]string{"skills": skillLog.String(), "items": itemLog.String()} {
		if strings.Contains(log, errTableRefNotAllowed.Error()) {
			t.Errorf("shipped %s: a table reference was rejected: %s", name, log)
		}
	}

	checked := 0
	var walk func(where string, c skill.Condition)
	walk = func(where string, c skill.Condition) {
		for k, v := range c.Attrs {
			checked++
			if strings.HasPrefix(v, "#") {
				t.Errorf("%s: <%s %s=%q> left unresolved", where, c.Kind, k, v)
			}
		}
		for _, ch := range c.Children {
			walk(where, ch)
		}
	}
	clause := func(where string, c *skill.ConditionClause) {
		if c == nil {
			return
		}
		if strings.HasPrefix(c.Message, "#") {
			t.Errorf("%s: cond msg %q", where, c.Message)
		}
		walk(where, c.Root)
	}
	funcs := func(where string, fs []skill.FuncTemplate) {
		for _, f := range fs {
			clause(where, f.AttachCondition)
			if f.Condition != nil {
				walk(where, *f.Condition)
			}
		}
	}
	for _, def := range skills.All() {
		where := fmt.Sprintf("skill %d level %d", def.ID, def.Level)
		for i := range def.Conditions {
			clause(where, &def.Conditions[i])
		}
		funcs(where, def.Funcs)
		for _, e := range append(append([]skill.EffectTemplate(nil), def.Effects...), def.SelfEffects...) {
			clause(where, e.AttachCondition)
			funcs(where, e.Funcs)
		}
	}
	if checked == 0 {
		t.Fatal("no shipped skill condition attribute checked")
	}

	def, ok := skills.Get(2274, 3)
	if !ok {
		t.Fatal("skill 2274 level 3 not loaded")
	}
	pair := def.Conditions[0].Root.Children[1].Attrs["active_skill_id_lvl"]
	if pair != "1315,10" {
		t.Errorf("skill 2274 level 3 active_skill_id_lvl = %q, want the table's third row 1315,10", pair)
	}

	var walkItem func(id int32, c item.Condition)
	walkItem = func(id int32, c item.Condition) {
		for k, v := range c.Attrs {
			if strings.HasPrefix(v, "#") {
				t.Errorf("item %d: <%s %s=%q>", id, c.Kind, k, v)
			}
		}
		for _, ch := range c.Children {
			walkItem(id, ch)
		}
	}
	for _, tpl := range items.All() {
		for _, uc := range tpl.UseConditions {
			walkItem(tpl.ID, uc.Root)
		}
	}
}

// TestSkillTemplateReadsOnlyALeadingCond: a <cond> gates a <for> or
// <effect> block only as its first child. A later one is never read, so it
// attaches nothing and a table reference in it does not reject the skill.
func TestSkillTemplateReadsOnlyALeadingCond(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"for":    `<for><add stat="pAtk" val="1"/><cond><player hp="#t"/></cond><mul stat="pDef" val="2"/></for>`,
		"effect": `<for><effect name="Buff" val="0" time="1"><add stat="pAtk" val="1"/><cond><player level="#t"/></cond><mul stat="pDef" val="2"/></effect></for>`,
	}
	for name, body := range cases {
		def, ok := loadCondTableSkill(t, body).Get(1, 1)
		if !ok {
			t.Errorf("%s: skill not loaded", name)
			continue
		}
		funcs := def.Funcs
		if len(def.Effects) == 1 {
			if def.Effects[0].AttachCondition != nil {
				t.Errorf("%s: effect attach condition %+v, want none", name, def.Effects[0].AttachCondition)
			}
			funcs = def.Effects[0].Funcs
		}
		if len(funcs) != 2 {
			t.Fatalf("%s: %d funcs, want 2", name, len(funcs))
		}
		for _, f := range funcs {
			if f.AttachCondition != nil {
				t.Errorf("%s: func %s gated by %+v, want ungated", name, f.Stat, f.AttachCondition)
			}
		}
	}
}

// TestTemplateCondMessageFollowsReference: a <for> or <effect> block's
// leading <cond> reads msg first and msgId only without one, like a
// skill-level <cond>; a table reference in an unread msgId does not reject
// the skill (probe: attach condition message "x"/"y", message id 0).
func TestTemplateCondMessageFollowsReference(t *testing.T) {
	t.Parallel()
	forDef, ok := loadCondTableSkill(t, `<for><cond msg="x" msgId="#t"><player level="1"/></cond><add stat="pAtk" val="1"/></for>`).Get(1, 1)
	if !ok {
		t.Fatal("<for> cond: skill not loaded")
	}
	if c := forDef.Funcs[0].AttachCondition; c == nil || c.Message != "x" || c.MessageID != 0 {
		t.Errorf("<for> cond = %+v, want message \"x\" and no message id", c)
	}
	effDef, ok := loadCondTableSkill(t, `<for><effect name="Buff" val="0" time="1"><cond msg="y" msgId="#t"><player level="1"/></cond><add stat="pAtk" val="1"/></effect></for>`).Get(1, 1)
	if !ok {
		t.Fatal("<effect> cond: skill not loaded")
	}
	if c := effDef.Effects[0].Funcs[0].AttachCondition; c == nil || c.Message != "y" || c.MessageID != 0 {
		t.Errorf("<effect> cond = %+v, want message \"y\" and no message id", c)
	}
}
