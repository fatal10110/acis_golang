package xml

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/rs/zerolog"
)

// The expectations in this file are the reference loaders' output on the
// same fixtures (#3683): DocumentItem / DocumentSkill built from the aCis
// sources and run under OpenJDK 21, each file loaded on its own and printed
// as the loaded items "id(w=weight,p=price)" or skills "id-level(mp=mpConsume)"
// in file order.

const etc = `type="EtcItem"`

// validZone is a valid insidePoly zone element.
var validZone = polyZoneXML("zone")

// loadedItems loads body as one item file and renders the loaded items with
// ids 1 to 9 the way the reference output prints them.
func loadedItems(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	writeXMLFixture(t, filepath.Join(dir, "fixture.xml"), "<list>"+body+"</list>")
	table, err := LoadItemTemplates(dir, zerolog.Nop())
	if err != nil {
		t.Fatalf("LoadItemTemplates: %v", err)
	}
	var out []string
	for id := int32(1); id <= 9; id++ {
		if tpl, ok := table.Get(id); ok {
			out = append(out, fmt.Sprintf("%d(w=%d,p=%d)", id, tpl.Weight, tpl.ReferencePrice))
		}
	}
	return strings.Join(out, " ")
}

// loadedSkills loads a file holding valid skills 1 and 3 around skill 2,
// which carries body, and renders the loaded skills.
func loadedSkills(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	writeXMLFixture(t, filepath.Join(dir, "fixture.xml"), abortFile(`<skill id="2" name="b" levels="1">`, body))
	table, err := LoadSkillDefinitions(dir, zerolog.Nop())
	if err != nil {
		t.Fatalf("LoadSkillDefinitions: %v", err)
	}
	var out []string
	for id := skill.ID(1); id <= 3; id++ {
		if def, ok := table.Get(id, 1); ok {
			out = append(out, fmt.Sprintf("%d-1(mp=%d)", id, def.MPConsume))
		}
	}
	return strings.Join(out, " ")
}

type loadCase struct {
	name, body, want string
}

func runItemCases(t *testing.T, cases []loadCase) {
	t.Helper()
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := loadedItems(t, c.body); got != c.want {
				t.Errorf("loaded %q, want %q", got, c.want)
			}
		})
	}
}

// TestItemChildrenReadInDocumentOrder: an item's children are read in
// document order. The first <for> or <cond> creates the item, after which a
// <set> or <table> fails it, and a <set> reads only the tables before it.
func TestItemChildrenReadInDocumentOrder(t *testing.T) {
	t.Parallel()
	runItemCases(t, []loadCase{
		{
			name: "set after for",
			body: `<item id="1" ` + etc + ` name="a"><for><add stat="pAtk" val="1"/></for><set name="weight" val="3"/></item><item id="2" ` + etc + ` name="b"/>`,
			want: "2(w=0,p=0)",
		},
		{
			name: "table after cond",
			body: `<item id="1" ` + etc + ` name="a"><cond><player level="1"/></cond><table name="#w">7</table></item><item id="2" ` + etc + ` name="b"/>`,
			want: "2(w=0,p=0)",
		},
		{
			name: "set before its table",
			body: `<item id="1" ` + etc + ` name="a"><set name="weight" val="#w"/><table name="#w">7</table></item><item id="2" ` + etc + ` name="b"/>`,
			want: "2(w=0,p=0)",
		},
		{
			name: "cond, for and cond after the sets",
			body: `<item id="1" ` + etc + ` name="a"><set name="weight" val="2"/><cond><player level="1"/></cond><for><add stat="pAtk" val="1"/></for><cond><player level="2"/></cond></item>`,
			want: "1(w=2,p=0)",
		},
	})
}

// TestItemTablesCarryOverFailedItem: the tables an item declared before it
// failed stay visible to the next items of the file until one loads.
func TestItemTablesCarryOverFailedItem(t *testing.T) {
	t.Parallel()
	failW := `<item id="1" ` + etc + ` name="a"><table name="#w">7</table><set val="1"/></item>`
	useW := func(id int) string {
		return fmt.Sprintf(`<item id="%d" `+etc+` name="u"><set name="weight" val="#w"/></item>`, id)
	}
	runItemCases(t, []loadCase{
		{name: "after a failed set", body: failW + useW(2), want: "2(w=7,p=0)"},
		{
			name: "reset after a loaded item",
			body: `<item id="1" ` + etc + ` name="a"><table name="#w">7</table><set name="weight" val="#w"/></item>` + useW(2) + `<item id="3" ` + etc + ` name="c"/>`,
			want: "1(w=7,p=0) 3(w=0,p=0)",
		},
		{
			name: "id that does not parse fails before the tables",
			body: `<item id="x" ` + etc + ` name="a"><table name="#w">7</table></item>` + useW(2) + `<item id="3" ` + etc + ` name="c"/>`,
			want: "3(w=0,p=0)",
		},
		{
			name: "missing type fails before the tables",
			body: `<item id="1" name="a"><table name="#w">7</table></item>` + useW(2) + `<item id="3" ` + etc + ` name="c"/>`,
			want: "3(w=0,p=0)",
		},
		{
			name: "unknown type fails after the tables",
			body: `<item id="1" type="Bogus" name="a"><table name="#w">7</table></item>` + useW(2),
			want: "2(w=7,p=0)",
		},
		{
			name: "failure in a for block",
			body: `<item id="1" ` + etc + ` name="a"><table name="#w">7</table><for><add stat="pAtk" val="#nope"/></for></item>` + useW(2),
			want: "2(w=7,p=0)",
		},
		{
			name: "failed value when the item is created",
			body: `<item id="1" ` + etc + ` name="a"><table name="#w">7</table><set name="weight" val="oops"/></item>` + useW(2),
			want: "2(w=7,p=0)",
		},
		{
			name: "tables after the failing node are not read",
			body: `<item id="1" ` + etc + ` name="a"><table name="#a">5</table><set name="weight" val="#missing"/><table name="#w">7</table></item>` +
				useW(2) + `<item id="3" ` + etc + ` name="c"><set name="weight" val="#a"/></item>`,
			want: "3(w=5,p=0)",
		},
		{
			name: "several failed items in a row",
			body: failW + `<item id="2" ` + etc + ` name="b"><table name="#p">9</table><set val="1"/></item>` +
				`<item id="3" ` + etc + ` name="c"><set name="weight" val="#w"/><set name="price" val="#p"/></item>` + useW(4),
			want: "3(w=7,p=9)",
		},
		{
			name: "own table overrides a carried one",
			body: failW + `<item id="2" ` + etc + ` name="b"><table name="#w">8</table><set name="weight" val="#w"/></item>`,
			want: "2(w=8,p=0)",
		},
		{
			name: "for reads a carried table",
			body: `<item id="1" ` + etc + ` name="a"><table name="#v">4</table><set val="1"/></item><item id="2" ` + etc + ` name="b"><for><add stat="pAtk" val="#v"/></for></item>`,
			want: "2(w=0,p=0)",
		},
		{
			name: "other elements between the items",
			body: failW + `<other/>` + useW(2),
			want: "2(w=7,p=0)",
		},
	})
}

// TestItemValueSetSeed: an item's values start from its id (as item_id) and
// name only; its other attributes are never read, a <set> of item_id renames
// it, and one of id or type changes nothing.
func TestItemValueSetSeed(t *testing.T) {
	t.Parallel()
	runItemCases(t, []loadCase{
		{name: "extra attribute", body: `<item id="1" ` + etc + ` name="a" weight="5"/>`, want: "1(w=0,p=0)"},
		{name: "set item_id", body: `<item id="1" ` + etc + ` name="a"><set name="item_id" val="9"/></item>`, want: "9(w=0,p=0)"},
		{name: "set id", body: `<item id="1" ` + etc + ` name="a"><set name="id" val="9"/></item>`, want: "1(w=0,p=0)"},
		{name: "set type", body: `<item id="1" ` + etc + ` name="a"><set name="type" val="Bogus"/></item>`, want: "1(w=0,p=0)"},
		{name: "signed id", body: `<item id="+1" ` + etc + ` name="a"/>`, want: "1(w=0,p=0)"},
	})
}

// TestItemCDATANodes: a CDATA section is a child node of its own, even when
// empty: a <table> reads only its first node, and a <player insidePoly> takes
// its second node as the zone.
func TestItemCDATANodes(t *testing.T) {
	t.Parallel()
	table := func(content string) string {
		return `<item id="1" ` + etc + ` name="a"><table name="#w">` + content + `</table><set name="weight" val="#w"/></item>`
	}
	poly := func(content string) string {
		return `<item id="1" ` + etc + ` name="a"><cond><player insidePoly="true">` + content + `</player></cond></item><item id="2" ` + etc + ` name="b"/>`
	}
	runItemCases(t, []loadCase{
		{name: "table text then CDATA", body: table(`7<![CDATA[8]]>`), want: "1(w=7,p=0)"},
		{name: "table CDATA then text", body: table(`<![CDATA[7]]>8`), want: "1(w=7,p=0)"},
		{name: "table empty CDATA then text", body: table(`<![CDATA[]]>7`) + `<item id="2" ` + etc + ` name="b"/>`, want: "2(w=0,p=0)"},
		{name: "table two CDATA sections", body: table(`<![CDATA[7]]><![CDATA[8]]>`), want: "1(w=7,p=0)"},
		{name: "table comment, CDATA, text", body: table(`<!-- c --><![CDATA[7]]>8`), want: "1(w=7,p=0)"},
		{name: "table character reference then CDATA", body: table(`&#55;<![CDATA[8]]>`), want: "1(w=7,p=0)"},
		{name: "table CDATA, comment, text", body: table(`<![CDATA[7]]><!-- c -->8`), want: "1(w=7,p=0)"},
		{name: "table text, comment, CDATA", body: table(`7<!-- c --><![CDATA[8]]>`), want: "1(w=7,p=0)"},
		{
			name: "table text, space, CDATA",
			body: `<item id="1" ` + etc + ` name="a"><table name="#w">7 <![CDATA[8]]></table><set name="weight" val="#w"/><set name="price" val="#w"/></item>`,
			want: "1(w=7,p=7)",
		},
		{name: "insidePoly CDATA then zone", body: poly(`<![CDATA[x]]>` + validZone), want: "1(w=0,p=0) 2(w=0,p=0)"},
		{name: "insidePoly text, CDATA, zone", body: poly(` <![CDATA[x]]>` + validZone), want: "2(w=0,p=0)"},
		{name: "insidePoly CDATA, text, zone", body: poly(`<![CDATA[x]]> ` + validZone), want: "2(w=0,p=0)"},
		{name: "insidePoly empty CDATA then zone", body: poly(`<![CDATA[]]>` + validZone), want: "1(w=0,p=0) 2(w=0,p=0)"},
		{name: "insidePoly empty CDATA, text, zone", body: poly(`<![CDATA[]]> ` + validZone), want: "2(w=0,p=0)"},
		{name: "insidePoly CDATA, comment, CDATA, zone", body: poly(`<![CDATA[x]]><!-- c --><![CDATA[y]]>` + validZone), want: "2(w=0,p=0)"},
		{
			name: "for leading empty CDATA keeps its cond unread",
			body: `<item id="1" ` + etc + ` name="a"><for><![CDATA[]]><cond><player level="oops"/></cond><add stat="pAtk" val="1"/></for></item><item id="2" ` + etc + ` name="b"/>`,
			want: "1(w=0,p=0) 2(w=0,p=0)",
		},
	})
}

// TestSkillCDATANodes: the same node boundaries hold in a skill file, where a
// failed skill stops the file.
func TestSkillCDATANodes(t *testing.T) {
	t.Parallel()
	poly := func(content string) string {
		return `<cond><player insidePoly="true">` + content + `</player></cond>`
	}
	cases := []loadCase{
		{name: "table text then CDATA", body: `<table name="#mp">7<![CDATA[8]]></table><set name="mpConsume" val="#mp"/>`, want: "1-1(mp=0) 2-1(mp=7) 3-1(mp=0)"},
		{name: "table CDATA then text", body: `<table name="#mp"><![CDATA[7]]>8</table><set name="mpConsume" val="#mp"/>`, want: "1-1(mp=0) 2-1(mp=7) 3-1(mp=0)"},
		{name: "table empty CDATA then text", body: `<table name="#mp"><![CDATA[]]>7</table><set name="mpConsume" val="#mp"/>`, want: "1-1(mp=0) 3-1(mp=0)"},
		{name: "table CDATA with a line break", body: "<table name=\"#mp\"><![CDATA[7\r\n8]]></table><set name=\"mpConsume\" val=\"#mp\"/>", want: "1-1(mp=0) 2-1(mp=7) 3-1(mp=0)"},
		{name: "insidePoly text, CDATA, zone", body: poly(` <![CDATA[x]]>` + validZone), want: "1-1(mp=0)"},
		{name: "insidePoly CDATA then zone", body: poly(`<![CDATA[x]]>` + validZone), want: "1-1(mp=0) 2-1(mp=0) 3-1(mp=0)"},
		{name: "insidePoly CDATA, text, zone", body: poly(`<![CDATA[x]]> ` + validZone), want: "1-1(mp=0)"},
		{name: "insidePoly empty CDATA then zone", body: poly(`<![CDATA[]]>` + validZone), want: "1-1(mp=0) 2-1(mp=0) 3-1(mp=0)"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := loadedSkills(t, c.body); got != c.want {
				t.Errorf("loaded %q, want %q", got, c.want)
			}
		})
	}
}
