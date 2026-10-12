package xml

import (
	encxml "encoding/xml"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/rs/zerolog"
)

// The expectations in this file come from #3392 and from loading the same
// fixtures with the reference loaders (DocumentSkill / DocumentItem built
// from the aCis sources, OpenJDK 21), whose parser ignores comments and does
// not coalesce: a <set> attribute that is missing reads as null;
// a <table> reads only its first child node's value, which is null for an
// element and absent for an empty table; an ignored comment joins the
// character data around it into one node; and a <player insidePoly> takes
// its second child node as the zone, which is null when nothing precedes the
// zone and has no attributes when it is character data or a processing
// instruction. Each null or missing attribute fails the skill (and its file)
// or the item.

// polyZoneXML is a valid zone element named tag.
func polyZoneXML(tag string) string {
	return `<` + tag + ` minZ="0" maxZ="10"><node x="0" y="0"/><node x="10" y="0"/><node x="10" y="10"/></` + tag + `>`
}

// TestSkillFileStopsAtUnreadableNode covers #3392's skill side: each case
// puts the failure in skill 2 between a valid skill 1 and a valid skill 3.
func TestSkillFileStopsAtUnreadableNode(t *testing.T) {
	t.Parallel()
	enchanted := `<skill id="2" name="b" levels="1" enchantLevels1="1" enchantLevels2="1">`
	cases := []struct {
		name, open, body string
	}{
		{name: "set without name", body: `<set val="1"/>`},
		{name: "set without val", body: `<set name="mpConsume"/>`},
		{name: "set tag in upper case without val", body: `<SET name="mpConsume"/>`},
		{name: "enchant1 without val", open: enchanted, body: `<enchant1 name="mpConsume"/>`},
		{name: "enchant2 without name", open: enchanted, body: `<Enchant2 val="1"/>`},
		{name: "self-closed table", body: `<table name="#x"/>`},
		{name: "table without content", body: `<table name="#x"></table>`},
		{name: "table holding only a comment", body: `<table name="#x"><!-- none --></table>`},
		{name: "table starting with an element", body: `<table name="#x"><v/>1 2</table>`},
		{name: "table tag in upper case without content", body: `<TABLE name="#x"/>`},
		{name: "insidePoly zone as the first node", body: `<cond><player insidePoly="true">` + polyZoneXML("zone") + `</player></cond>`},
		{name: "insidePoly zone after only a comment", body: `<cond><player insidePoly="true"><!-- c -->` + polyZoneXML("zone") + `</player></cond>`},
		{name: "insidePoly zone after text and a processing instruction", body: `<cond><player insidePoly="true"> <?pi x?>` + polyZoneXML("zone") + `</player></cond>`},
		{name: "insidePoly second node is text", body: `<cond><player insidePoly="true">` + polyZoneXML("zone") + ` </player></cond>`},
		{name: "insidePoly second node is an element without minZ", body: "<cond><player insidePoly=\"true\">\n<a/>" + polyZoneXML("zone") + `</player></cond>`},
		{name: "insidePoly in a func condition", body: `<for><add stat="pAtk" val="1"><player insidePoly="true">` + polyZoneXML("zone") + `</player></add></for>`},
		{name: "cond tag in upper case", body: `<COND><player level="oops"/></COND>`},
		{name: "for tag in mixed case", body: `<For><add stat="pAtk" val="x"/></For>`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			writeXMLFixture(t, filepath.Join(dir, "fixture.xml"), abortFile(c.open, c.body))
			table, err := LoadSkillDefinitions(dir, zerolog.Nop())
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
		})
	}
}

// TestSkillFileReadsTolerableNodes checks the shapes the reader takes: sets
// it never reads, a table whose first node is blank, and an insidePoly zone
// wherever the second child node is an element.
func TestSkillFileReadsTolerableNodes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, open, body string
	}{
		{name: "set without val in a skill without levels", open: `<skill id="2" name="b" levels="0">`, body: `<set name="mpConsume"/>`},
		{name: "enchant1 without val and no enchant1 levels", body: `<enchant1 name="mpConsume"/>`},
		{name: "blank table", body: `<table name="#x"> </table>`},
		{name: "table after a comment", body: `<table name="#x"><!-- c -->1</table>`},
		{name: "insidePoly zone after a line break", body: "<cond><player insidePoly=\"true\">\n" + polyZoneXML("zone") + `</player></cond>`},
		{name: "insidePoly zone after text, a comment and text", body: "<cond><player insidePoly=\"true\">\n<!-- c -->\n" + polyZoneXML("zone") + `</player></cond>`},
		{name: "insidePoly zone after a processing instruction", body: `<cond><player insidePoly="true"><?pi x?>` + polyZoneXML("zone") + `</player></cond>`},
		{name: "insidePoly zone after another element", body: `<cond><player insidePoly="true"><a/>` + polyZoneXML("zone") + `</player></cond>`},
		{name: "insidePoly zone is the second node, not the first element", body: `<cond><player insidePoly="true"><a minZ="1"/>` + polyZoneXML("b") + `</player></cond>`},
		{name: "insidePoly zone of any tag", body: `<cond><player insidePoly="true"> ` + polyZoneXML("area") + `</player></cond>`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			writeXMLFixture(t, filepath.Join(dir, "fixture.xml"), abortFile(c.open, c.body))
			table, err := LoadSkillDefinitions(dir, zerolog.Nop())
			if err != nil {
				t.Fatalf("LoadSkillDefinitions: %v", err)
			}
			for _, id := range []skill.ID{1, 3} {
				if _, ok := table.Get(id, 1); !ok {
					t.Fatalf("skill %d not loaded", id)
				}
			}
		})
	}
}

// TestSkillInsidePolyKeepsItsZone checks that the zone a <player insidePoly>
// reads is the one its condition holds, as its only child.
func TestSkillInsidePolyKeepsItsZone(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeXMLFixture(t, filepath.Join(dir, "fixture.xml"), skillFixture(
		`<cond><player insidePoly="true"><a minZ="1" maxZ="2"/><b minZ="-5" maxZ="5"><node x="0" y="0"/><node x="9" y="0"/><node x="9" y="9"/></b></player></cond>`))
	table, err := LoadSkillDefinitions(dir, zerolog.Nop())
	if err != nil {
		t.Fatalf("LoadSkillDefinitions: %v", err)
	}
	def, ok := table.Get(1, 1)
	if !ok || len(def.Conditions) != 1 {
		t.Fatalf("skill 1 = %v, conditions %+v; want one condition", ok, def.Conditions)
	}
	zones := def.Conditions[0].Root.Children
	if len(zones) != 1 || zones[0].Kind != "b" || zones[0].Attrs["minZ"] != "-5" || len(zones[0].Children) != 3 {
		t.Fatalf("insidePoly children = %+v, want only <b minZ=-5> with three nodes", zones)
	}
}

// TestSkillTablesReadFirstNodeOnly checks a table row's values: only the
// first child node counts, a comment joins the text around it, and only
// space, tab, line feed, carriage return and form feed separate values.
func TestSkillTablesReadFirstNodeOnly(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, table string
		want        []string
	}{
		{name: "text split by a comment", table: `1 <!-- c --> 2<!-- d -->3`, want: []string{"1", "23"}},
		{name: "text before an element", table: `4 5<v/>6`, want: []string{"4", "5"}},
		{name: "processing instruction first", table: `<?row 7 8?>9`, want: []string{"7", "8"}},
		{name: "white space that does not separate", table: "a\u00a0b\u2003c\td", want: []string{"a\u00a0b\u2003c", "d"}},
		{name: "blank", table: " \n ", want: nil},
	}
	for _, c := range cases {
		var tbl tableElement
		if err := encxml.Unmarshal([]byte(`<table name="#t">`+c.table+`</table>`), &tbl); err != nil {
			t.Fatalf("%s: decode: %v", c.name, err)
		}
		tables, err := buildValueTables([]tableElement{tbl})
		if err != nil {
			t.Fatalf("%s: buildValueTables: %v", c.name, err)
		}
		if got := tables["#t"]; !slices.Equal(got, c.want) {
			t.Errorf("%s: row = %q, want %q", c.name, got, c.want)
		}
	}
}

// TestSkillTagsMatchIgnoringCase checks that every tag the skill reader
// looks for matches ignoring case, from the root <list> down.
func TestSkillTagsMatchIgnoringCase(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeXMLFixture(t, filepath.Join(dir, "fixture.xml"), `<LIST>
		<Skill id="1" name="x" levels="1" enchantLevels1="1" enchantLevels2="1">
			<TABLE name="#mp">12</TABLE>
			<Set name="target" val="ONE"/><SET name="skillType" val="PDAM"/><set name="operateType" val="ACTIVE"/>
			<sEt name="mpConsume" val="#mp"/>
			<ENCHANT1 name="mpConsume" val="20"/><Enchant2 name="mpConsume" val="30"/>
			<COND msg="no"><player level="1"/></COND>
			<For><add stat="pAtk" val="1"/></For>
			<Enchant1Cond><player level="2"/></Enchant1Cond><ENCHANT1FOR><add stat="pAtk" val="2"/></ENCHANT1FOR>
			<enchant2COND><player level="3"/></enchant2COND><Enchant2For><add stat="pAtk" val="3"/></Enchant2For>
		</Skill>
	</LIST>`)
	table, err := LoadSkillDefinitions(dir, zerolog.Nop())
	if err != nil {
		t.Fatalf("LoadSkillDefinitions: %v", err)
	}
	for _, c := range []struct {
		level, mp int
		lvl       string
		pAtk      float64
	}{{1, 12, "1", 1}, {101, 20, "2", 2}, {141, 30, "3", 3}} {
		def, ok := table.Get(1, c.level)
		if !ok {
			t.Fatalf("level %d not loaded", c.level)
		}
		if def.MPConsume != c.mp {
			t.Errorf("level %d MPConsume = %d, want %d", c.level, def.MPConsume, c.mp)
		}
		if len(def.Conditions) != 1 || def.Conditions[0].Root.Attrs["level"] != c.lvl {
			t.Errorf("level %d conditions = %+v, want one with level %s", c.level, def.Conditions, c.lvl)
		}
		if len(def.Funcs) != 1 || def.Funcs[0].Value != c.pAtk {
			t.Errorf("level %d funcs = %+v, want one pAtk %v", c.level, def.Funcs, c.pAtk)
		}
	}
}

// TestSkillFileRoots checks which roots a skill file loads from: a <list>
// or a lone <skill>, in any case, and nothing under any other root.
func TestSkillFileRoots(t *testing.T) {
	t.Parallel()
	one := `<skill id="1" name="x" levels="1">` + abortSets + `</skill>`
	cases := []struct {
		name, content string
		loaded        bool
	}{
		{name: "list", content: `<list>` + one + `</list>`, loaded: true},
		{name: "list in upper case", content: `<LIST>` + one + `</LIST>`, loaded: true},
		{name: "root skill", content: `<?xml version="1.0"?>` + "\n" + one, loaded: true},
		{name: "root skill in upper case", content: `<SKILL id="1" name="x" levels="1">` + abortSets + `</SKILL>`, loaded: true},
		{name: "other root", content: `<skills>` + one + `</skills>`},
	}
	for _, c := range cases {
		dir := t.TempDir()
		writeXMLFixture(t, filepath.Join(dir, "fixture.xml"), c.content)
		table, err := LoadSkillDefinitions(dir, zerolog.Nop())
		if err != nil {
			t.Fatalf("%s: LoadSkillDefinitions: %v", c.name, err)
		}
		if _, ok := table.Get(1, 1); ok != c.loaded {
			t.Errorf("%s: skill 1 loaded = %v, want %v", c.name, ok, c.loaded)
		}
	}
}

// TestSkillSetTrimsOnlyControlSpace checks that a <set> name and val lose
// leading and trailing spaces and control characters only: a no-break space
// stays, so the level's integer does not parse and the level is dropped.
func TestSkillSetTrimsOnlyControlSpace(t *testing.T) {
	t.Parallel()
	cases := []struct {
		val    string
		loaded bool
	}{{val: "&#9; 5 &#10;", loaded: true}, {val: "&#160;5"}}
	for _, c := range cases {
		dir := t.TempDir()
		writeXMLFixture(t, filepath.Join(dir, "fixture.xml"), skillFixture(`<set name="&#9;mpConsume " val="`+c.val+`"/>`))
		table, err := LoadSkillDefinitions(dir, zerolog.Nop())
		if err != nil {
			t.Fatalf("LoadSkillDefinitions: %v", err)
		}
		def, ok := table.Get(1, 1)
		if ok != c.loaded || (ok && def.MPConsume != 5) {
			t.Errorf("val %q: level 1 loaded = %v (MPConsume %d), want loaded = %v with 5", c.val, ok, def.MPConsume, c.loaded)
		}
	}
}

// TestItemSkippedAtUnreadableNode covers #3392's item side: the item is
// skipped, the items around it load.
func TestItemSkippedAtUnreadableNode(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"set without name":                    `<set val="1"/>`,
		"set without val":                     `<set name="weight"/>`,
		"set tag in upper case without val":   `<SET name="weight"/>`,
		"self-closed table":                   `<table name="#x"/>`,
		"table starting with an element":      `<table name="#x"><v/>1</table>`,
		"insidePoly zone as the first node":   `<cond><player insidePoly="true">` + polyZoneXML("zone") + `</player></cond>`,
		"insidePoly second node is text":      `<cond><player insidePoly="true">` + polyZoneXML("zone") + ` </player></cond>`,
		"cond tag in upper case":              `<COND><player level="oops"/></COND>`,
		"for tag in upper case":               `<FOR><add stat="pAtk" val="x"/></FOR>`,
		"table tag in upper case, no content": `<Table name="#x"></Table>`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			writeItemFile(t, dir, "fixture.xml", `
				<item id="1" type="EtcItem" name="a"/>
				<item id="2" type="Weapon" name="b"><set name="bodypart" val="rhand"/>`+body+`</item>
				<item id="3" type="EtcItem" name="c"/>`)
			table, err := LoadItemTemplates(dir, zerolog.Nop())
			if err != nil {
				t.Fatalf("LoadItemTemplates: %v", err)
			}
			if _, ok := table.Get(2); ok {
				t.Fatal("item 2 loaded, want it skipped")
			}
			for _, id := range []int32{1, 3} {
				if _, ok := table.Get(id); !ok {
					t.Fatalf("item %d not loaded", id)
				}
			}
		})
	}
}

// TestItemTagsMatchIgnoringCase checks an item read through tags in any
// case, its <set> values trimmed, and its insidePoly zone read from the
// second child node.
func TestItemTagsMatchIgnoringCase(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	content := "<LIST><Item id=\"5\" type=\"EtcItem\" name=\"e\">" +
		`<Table name="#w">7</Table><SET name=" weight " val=" #w "/><Set name="price" val="9"/>` +
		`<Cond msgId="113"><player insidePoly="true"><a/>` + polyZoneXML("zone") + `</player></Cond>` +
		`<FOR><add stat="pAtk" val="4"/></FOR>` +
		`</Item></LIST>`
	if err := os.WriteFile(filepath.Join(dir, "fixture.xml"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	table, err := LoadItemTemplates(dir, zerolog.Nop())
	if err != nil {
		t.Fatalf("LoadItemTemplates: %v", err)
	}
	tpl, ok := table.Get(5)
	if !ok {
		t.Fatal("item 5 not loaded")
	}
	if tpl.Weight != 7 || tpl.ReferencePrice != 9 {
		t.Errorf("weight, price = %d, %d; want 7, 9", tpl.Weight, tpl.ReferencePrice)
	}
	if len(tpl.Modifiers) != 1 || tpl.Modifiers[0].Value != 4 {
		t.Errorf("modifiers = %+v, want one pAtk 4", tpl.Modifiers)
	}
	if len(tpl.UseConditions) != 1 {
		t.Fatalf("use conditions = %+v, want one", tpl.UseConditions)
	}
	zones := tpl.UseConditions[0].Root.Children
	if len(zones) != 1 || zones[0].Kind != "zone" || zones[0].Attrs["maxZ"] != "10" {
		t.Errorf("insidePoly children = %+v, want only the zone", zones)
	}
}

// TestItemFileNeedsListRoot checks that an item file loads only from a root
// <list>, in any case.
func TestItemFileNeedsListRoot(t *testing.T) {
	t.Parallel()
	one := `<item id="1" type="EtcItem" name="a"/>`
	for content, loaded := range map[string]bool{
		`<List>` + one + `</List>`:   true,
		`<items>` + one + `</items>`: false,
		one:                          false,
	} {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "fixture.xml"), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		table, err := LoadItemTemplates(dir, zerolog.Nop())
		if err != nil {
			t.Fatalf("%s: LoadItemTemplates: %v", content, err)
		}
		if _, ok := table.Get(1); ok != loaded {
			t.Errorf("%s: item 1 loaded = %v, want %v", content, ok, loaded)
		}
	}
}
