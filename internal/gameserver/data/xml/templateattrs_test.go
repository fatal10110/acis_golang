package xml

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/rs/zerolog"
)

// TestStarterItemAndSkillGrantAttrs pins the class template's <item> and
// <skill> readers: every listed attribute is required except isEquipped,
// which defaults to true, and a present int must parse.
func TestStarterItemAndSkillGrantAttrs(t *testing.T) {
	t.Parallel()
	item := attrSet{"id": "1147", "count": "1"}
	if got, err := buildStarterItem(item.values()); err != nil || got != (player.StarterItem{ItemID: 1147, Count: 1, Equipped: true}) {
		t.Errorf("buildStarterItem = %+v, %v", got, err)
	}
	if got, err := buildStarterItem(item.with("isEquipped", "False").values()); err != nil || got.Equipped {
		t.Errorf("buildStarterItem isEquipped=False = %+v, %v", got, err)
	}
	grant := attrSet{"id": "194", "lvl": "1", "minLvl": "1", "cost": "-1"}
	if got, err := buildSkillGrant(grant.values()); err != nil || got != (player.SkillGrant{SkillID: 194, Level: 1, MinLevel: 1, Cost: -1}) {
		t.Errorf("buildSkillGrant = %+v, %v", got, err)
	}

	for _, b := range []struct {
		name  string
		valid attrSet
		keys  []string
		build func(*attrValues) error
	}{
		{"starter item", item, []string{"id", "count"}, func(a *attrValues) error { _, err := buildStarterItem(a); return err }},
		{"skill grant", grant, []string{"id", "lvl", "minLvl", "cost"}, func(a *attrValues) error { _, err := buildSkillGrant(a); return err }},
	} {
		for _, key := range b.keys {
			if err := b.build(b.valid.without(key).values()); err == nil {
				t.Errorf("%s: missing %s built, want an error", b.name, key)
			}
			for _, bad := range []string{"", " 1", "1 ", "x"} {
				if err := b.build(b.valid.with(key, bad).values()); err == nil {
					t.Errorf("%s: %s=%q built, want an error", b.name, key, bad)
				}
			}
		}
	}
}

// TestClassTemplateTableAttrs: a stat table is a ";" list of floats in which
// an empty or malformed entry fails the template, and safeFallHeight must
// hold exactly two ints.
func TestClassTemplateTableAttrs(t *testing.T) {
	t.Parallel()
	const class = `<list><class>
		<set id="0" baseLvl="1" fists="1"/>
		<set pAtk="1" pDef="1" mAtk="1" mDef="1"/>
		<set radius="1" radiusFemale="1" height="1" heightFemale="1"/>
		<set safeFallHeight="SAFE"/>
		<set hpTable="HP" mpTable="1" cpTable="1"/>
		<set hpRegenTable="1" mpRegenTable="1" cpRegenTable="1"/>
	</class></list>`
	load := func(safeFall, hp string) (*player.Template, error) {
		dir := t.TempDir()
		writeXMLFixture(t, filepath.Join(dir, "test.xml"), strings.NewReplacer("SAFE", safeFall, "HP", hp).Replace(class))
		table, err := LoadPlayerTemplates(dir)
		if err != nil {
			return nil, err
		}
		tmpl, _ := table.Get(0)
		return tmpl, nil
	}

	tmpl, err := load("333;400", "80.5;12.25")
	if err != nil {
		t.Fatalf("LoadPlayerTemplates: %v", err)
	}
	if tmpl.SafeFallHeightFemale != 333 || tmpl.SafeFallHeightMale != 400 || len(tmpl.HPTable) != 2 || tmpl.HPTable[0] != 80.5 {
		t.Errorf("template safe fall %d/%d hp table %v", tmpl.SafeFallHeightFemale, tmpl.SafeFallHeightMale, tmpl.HPTable)
	}
	for _, tc := range []struct{ safeFall, hp string }{
		{"333;400", "80.5;;81"},
		{"333;400", "80.5;x"},
		{"333;400", ""},
		{"333", "1"},
		{"333;400;500", "1"},
		{"333;x", "1"},
	} {
		if _, err := load(tc.safeFall, tc.hp); err == nil {
			t.Errorf("safeFallHeight=%q hpTable=%q loaded, want an error", tc.safeFall, tc.hp)
		}
	}
}

// TestDoorTemplateAttrs pins the door reader: the position is an integer
// literal while the outline and stats stay plain decimal, type and openType
// must name a known kind, openType defaults to NPC, and a blank name fails.
func TestDoorTemplateAttrs(t *testing.T) {
	t.Parallel()
	const doc = `<list><door id="1" type="TYPE" level="1" name="NAME"><position x="0x10" y="#20" z="-010"/>` +
		`<coordinates><loc x="1" y="2"/><loc x="1" y="3"/><loc x="2" y="3"/></coordinates>` +
		`<stats hp="5" pDef="6" mDef="7" height="8"/><function OPEN/></door></list>`
	load := func(kind, name, open string) (*door.Template, error) {
		path := filepath.Join(t.TempDir(), "doors.xml")
		writeXMLFixture(t, path, strings.NewReplacer("TYPE", kind, "NAME", name, "OPEN", open).Replace(doc))
		table, err := LoadDoors(path, zerolog.Nop())
		if err != nil {
			return nil, err
		}
		tmpl, _ := table.Get(1)
		return tmpl, nil
	}

	tmpl, err := load("WALL", "gate", `castle="3" opened="TRUE" openTime="60"`)
	if err != nil {
		t.Fatalf("LoadDoors: %v", err)
	}
	if tmpl.Kind != door.KindWall || tmpl.Position.X != 16 || tmpl.Position.Y != 32 || tmpl.Position.Z != -8 ||
		tmpl.HP != 5 || tmpl.PDef != 6 || tmpl.MDef != 7 || tmpl.Height != 8 ||
		tmpl.CastleID != 3 || !tmpl.Opened || tmpl.OpenKind != door.OpenNPC || tmpl.OpenTime != 60 {
		t.Errorf("door template = %+v", *tmpl)
	}
	if tmpl, err := load("DOOR", "gate", `openType="CLICK"`); err != nil || tmpl.OpenKind != door.OpenClick {
		t.Errorf("openType=CLICK: %+v, %v", tmpl, err)
	}
	for _, tc := range []struct{ kind, name, open string }{
		{"door", "gate", ""},
		{"DOOR", "", ""},
		{"DOOR", "gate", `openType="click"`},
		{"DOOR", "gate", `castle="x"`},
	} {
		if _, err := load(tc.kind, tc.name, tc.open); err == nil {
			t.Errorf("type=%q name=%q %s loaded, want an error", tc.kind, tc.name, tc.open)
		}
	}
}
