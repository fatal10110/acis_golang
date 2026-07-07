package xml

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// writeItemFile creates name under dir with body wrapped in a <list> root,
// mirroring how a shipped item template file is shaped.
func writeItemFile(t *testing.T, dir, name, body string) {
	t.Helper()
	content := "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<list>\n" + body + "\n</list>\n"
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLoadItemTemplates(t *testing.T) {
	dir := t.TempDir()

	writeItemFile(t, dir, "0000-0099.xml", `
	<item id="10" type="Weapon" name="Dagger">
		<set name="default_action" val="equip" />
		<set name="weapon_type" val="DAGGER" />
		<set name="bodypart" val="rhand" />
		<set name="price" val="138" />
		<set name="weight" val="1160" />
		<set name="material" val="FINE_STEEL" />
		<set name="crystal_type" val="D" />
		<set name="crystal_count" val="10" />
		<set name="duration" val="60" />
		<set name="is_tradable" val="false" />
		<set name="soulshots" val="1" />
		<set name="spiritshots" val="1" />
		<set name="random_damage" val="5" />
		<set name="mp_consume" val="4" />
		<set name="mp_consume_reduce" val="10,2" />
		<set name="reuse_delay" val="500" />
		<set name="is_magical" val="true" />
		<set name="reduced_soulshot" val="30,1" />
		<set name="enchant4_skill" val="3013-1" />
		<set name="oncast_skill" val="3579-1" />
		<set name="oncast_chance" val="20" />
		<set name="oncrit_skill" val="3580-1" />
		<set name="item_skill" val="3599-1;3600-2" />
		<for>
			<set stat="pAtk" val="5" />
			<sub stat="accCombat" val="3" />
		</for>
		<cond msgId="100">
			<player level="40" />
		</cond>
	</item>
	<item id="11" type="Armor" name="Squire's Shirt">
		<set name="armor_type" val="LIGHT" />
		<set name="bodypart" val="chest" />
	</item>
	<item id="12" type="Armor" name="Basic Shield">
		<set name="bodypart" val="lhand" />
	</item>
	<item id="13" type="EtcItem" name="Custom Message Item">
		<cond msg="You may not use this." addName="1">
			<player level="10" />
		</cond>
	</item>
	<item id="14" type="EtcItem" name="AddName Item">
		<cond msgId="200" addName="1">
			<player level="10" />
		</cond>
	</item>
	<item id="15" type="EtcItem" name="And Cond Item">
		<cond msgId="300">
			<and>
				<player level="10" />
				<player sex="1" />
			</and>
		</cond>
	</item>`)

	writeItemFile(t, dir, "1400-1499.xml", `
	<item id="1400" type="EtcItem" name="Soulshot Sample">
		<set name="default_action" val="soulshot" />
		<set name="etcitem_type" val="SCROLL" />
	</item>`)

	writeItemFile(t, dir, "5500-5599.xml", `
	<item id="5588" type="EtcItem" name="Tutorial Guide">
		<set name="default_action" val="show_html" />
	</item>
	<item id="57" type="EtcItem" name="Adena">
		<set name="is_stackable" val="true" />
	</item>
	<item id="99" type="Potato" name="Bad Kind Item">
	</item>
	<item id="98" type="Weapon" name="Bad Slot Item">
		<set name="bodypart" val="tail" />
	</item>
	<item id="notanumber" type="Weapon" name="Bad Id Item">
	</item>`)

	log, hook := test.NewNullLogger()
	log.SetLevel(logrus.WarnLevel)

	table, err := LoadItemTemplates(dir, log)
	if err != nil {
		t.Fatalf("LoadItemTemplates: %v", err)
	}

	if got, want := table.Len(), 9; got != want {
		t.Fatalf("table.Len() = %d, want %d", got, want)
	}
	if got := len(hook.Entries); got != 3 {
		t.Fatalf("skipped-item warnings logged = %d, want 3", got)
	}
	for _, badID := range []int32{99, 98} {
		if _, ok := table.Get(badID); ok {
			t.Errorf("Get(%d): expected the malformed entry to be skipped", badID)
		}
	}

	t.Run("base fields and defaults", func(t *testing.T) {
		tpl, ok := table.Get(11)
		if !ok {
			t.Fatal("item 11 not loaded")
		}
		if tpl.Name != "Squire's Shirt" || tpl.Kind != item.KindArmor || tpl.Slot != item.SlotChest {
			t.Fatalf("item 11 identity = %+v", tpl)
		}
		if tpl.Weight != 0 || tpl.Material != item.MaterialSteel || tpl.Duration != -1 {
			t.Fatalf("item 11 defaults = %+v", tpl)
		}
		if !tpl.Sellable || !tpl.Dropable || !tpl.Destroyable || !tpl.Tradable || !tpl.Depositable {
			t.Fatalf("item 11 flag defaults = %+v", tpl)
		}
		if tpl.Stackable || tpl.OlyRestricted {
			t.Fatalf("item 11 flag defaults = %+v", tpl)
		}
		if tpl.Armor == nil || tpl.Armor.Type != item.ArmorLight {
			t.Fatalf("item 11 armor detail = %+v", tpl.Armor)
		}
	})

	t.Run("weapon detail and overrides", func(t *testing.T) {
		tpl, ok := table.Get(10)
		if !ok {
			t.Fatal("item 10 not loaded")
		}
		if tpl.Weight != 1160 || tpl.Material != item.MaterialFineSteel || tpl.ReferencePrice != 138 {
			t.Fatalf("item 10 base fields = %+v", tpl)
		}
		if tpl.Crystal != item.CrystalD || tpl.CrystalCount != 10 || tpl.Duration != 60 {
			t.Fatalf("item 10 crystal/duration = %+v", tpl)
		}
		if tpl.Tradable {
			t.Fatalf("item 10 Tradable = true, want false (explicit override)")
		}
		if !tpl.Sellable || !tpl.Dropable {
			t.Fatalf("item 10 unset flags should default true: %+v", tpl)
		}

		w := tpl.Weapon
		if w == nil {
			t.Fatal("item 10 Weapon detail is nil")
		}
		if w.Type != item.WeaponDagger || w.SoulshotCount != 1 || w.SpiritshotCount != 1 {
			t.Fatalf("item 10 weapon type/shots = %+v", w)
		}
		if w.RandomDamage != 5 || w.MPConsume != 4 || w.ReuseDelay != 500 || !w.Magical {
			t.Fatalf("item 10 weapon combat fields = %+v", w)
		}
		if w.MPConsumeReduceRate != 10 || w.MPConsumeReduceValue != 2 {
			t.Fatalf("item 10 mp consume reduce = %+v", w)
		}
		if w.ReducedSoulshotChance != 30 || w.ReducedSoulshotCount != 1 {
			t.Fatalf("item 10 reduced soulshot = %+v", w)
		}
		if w.Enchant4Skill == nil || *w.Enchant4Skill != (item.SkillRef{ID: 3013, Level: 1}) {
			t.Fatalf("item 10 enchant4 skill = %+v", w.Enchant4Skill)
		}
		if w.OnCastSkill == nil || w.OnCastSkill.Skill != (item.SkillRef{ID: 3579, Level: 1}) || w.OnCastSkill.Chance != 20 {
			t.Fatalf("item 10 on-cast skill = %+v", w.OnCastSkill)
		}
		if w.OnCritSkill == nil || w.OnCritSkill.Skill != (item.SkillRef{ID: 3580, Level: 1}) || w.OnCritSkill.Chance != -1 {
			t.Fatalf("item 10 on-crit skill (no chance set) = %+v", w.OnCritSkill)
		}

		wantSkills := []item.SkillRef{{ID: 3599, Level: 1}, {ID: 3600, Level: 2}}
		if len(tpl.AttachedSkills) != len(wantSkills) {
			t.Fatalf("item 10 AttachedSkills = %+v, want %+v", tpl.AttachedSkills, wantSkills)
		}
		for i, want := range wantSkills {
			if tpl.AttachedSkills[i] != want {
				t.Fatalf("item 10 AttachedSkills[%d] = %+v, want %+v", i, tpl.AttachedSkills[i], want)
			}
		}

		wantMods := []item.StatModifier{
			{Op: item.FuncSet, Stat: "pAtk", Value: 5},
			{Op: item.FuncSub, Stat: "accCombat", Value: 3},
		}
		if len(tpl.Modifiers) != len(wantMods) {
			t.Fatalf("item 10 Modifiers = %+v, want %+v", tpl.Modifiers, wantMods)
		}
		for i, want := range wantMods {
			if tpl.Modifiers[i] != want {
				t.Fatalf("item 10 Modifiers[%d] = %+v, want %+v", i, tpl.Modifiers[i], want)
			}
		}

		if len(tpl.UseConditions) != 1 {
			t.Fatalf("item 10 UseConditions = %+v, want 1 entry", tpl.UseConditions)
		}
		uc := tpl.UseConditions[0]
		if uc.MessageID != 100 || uc.Message != "" || uc.AddName {
			t.Fatalf("item 10 UseConditions[0] message fields = %+v", uc)
		}
		if uc.Root.Kind != "player" || uc.Root.Attrs["level"] != "40" {
			t.Fatalf("item 10 UseConditions[0] root = %+v", uc.Root)
		}
	})

	t.Run("armor without armor_type in the one-handed slot reports as a shield", func(t *testing.T) {
		tpl, ok := table.Get(12)
		if !ok {
			t.Fatal("item 12 not loaded")
		}
		if tpl.Armor == nil || tpl.Armor.Type != item.ArmorShield {
			t.Fatalf("item 12 armor detail = %+v, want ArmorShield", tpl.Armor)
		}
	})

	t.Run("etc item shot override beats an explicit etcitem_type", func(t *testing.T) {
		tpl, ok := table.Get(1400)
		if !ok {
			t.Fatal("item 1400 not loaded")
		}
		if tpl.EtcItem == nil || tpl.EtcItem.Type != item.EtcItemShot {
			t.Fatalf("item 1400 etc item detail = %+v, want EtcItemShot", tpl.EtcItem)
		}
	})

	t.Run("cond msg attribute wins over msgId and ignores addName", func(t *testing.T) {
		tpl, ok := table.Get(13)
		if !ok {
			t.Fatal("item 13 not loaded")
		}
		if len(tpl.UseConditions) != 1 {
			t.Fatalf("item 13 UseConditions = %+v", tpl.UseConditions)
		}
		uc := tpl.UseConditions[0]
		if uc.Message != "You may not use this." || uc.MessageID != 0 || uc.AddName {
			t.Fatalf("item 13 UseConditions[0] = %+v", uc)
		}
	})

	t.Run("cond msgId with addName", func(t *testing.T) {
		tpl, ok := table.Get(14)
		if !ok {
			t.Fatal("item 14 not loaded")
		}
		uc := tpl.UseConditions[0]
		if uc.MessageID != 200 || !uc.AddName {
			t.Fatalf("item 14 UseConditions[0] = %+v", uc)
		}
	})

	t.Run("cond predicate tree preserves combinator nesting", func(t *testing.T) {
		tpl, ok := table.Get(15)
		if !ok {
			t.Fatal("item 15 not loaded")
		}
		root := tpl.UseConditions[0].Root
		if root.Kind != "and" || len(root.Children) != 2 {
			t.Fatalf("item 15 UseConditions[0].Root = %+v", root)
		}
		if root.Children[0].Kind != "player" || root.Children[0].Attrs["level"] != "10" {
			t.Fatalf("item 15 UseConditions[0].Root.Children[0] = %+v", root.Children[0])
		}
		if root.Children[1].Kind != "player" || root.Children[1].Attrs["sex"] != "1" {
			t.Fatalf("item 15 UseConditions[0].Root.Children[1] = %+v", root.Children[1])
		}
	})
}

func TestLoadItemTemplatesMissingDirectory(t *testing.T) {
	if _, err := LoadItemTemplates(filepath.Join(t.TempDir(), "does-not-exist"), nil); err == nil {
		t.Fatal("LoadItemTemplates with missing directory: expected error")
	}
}

func TestLoadItemTemplatesMalformedXML(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "broken.xml"), []byte("<list><item id=\"1\">"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := LoadItemTemplates(dir, nil); err == nil {
		t.Fatal("LoadItemTemplates with malformed XML: expected error")
	}
}

// TestLoadItemTemplatesSkipsMalformedItems checks that a single <item>
// element with a data problem is logged and skipped rather than aborting
// the whole file's load, for every kind of problem this loader can detect
// beyond id/kind/slot (already covered by TestLoadItemTemplates).
func TestLoadItemTemplatesSkipsMalformedItems(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{
			name:    "unrecognized stat modifier element",
			content: `<item id="1" type="Weapon" name="x"><set name="bodypart" val="rhand"/><for><bogus stat="pAtk" val="1"/></for></item>`,
		},
		{
			name:    "non-numeric stat modifier value",
			content: `<item id="1" type="Weapon" name="x"><set name="bodypart" val="rhand"/><for><set stat="pAtk" val="notanumber"/></for></item>`,
		},
		{
			name:    "cond block with no predicate",
			content: `<item id="1" type="Weapon" name="x"><set name="bodypart" val="rhand"/><cond msgId="1"></cond></item>`,
		},
		{
			name:    "malformed item_skill reference",
			content: `<item id="1" type="Weapon" name="x"><set name="bodypart" val="rhand"/><set name="item_skill" val="notapair"/></item>`,
		},
		{
			name:    "malformed mp_consume_reduce pair",
			content: `<item id="1" type="Weapon" name="x"><set name="bodypart" val="rhand"/><set name="mp_consume_reduce" val="1,2,3"/></item>`,
		},
		{
			name:    "malformed enchant4_skill reference",
			content: `<item id="1" type="Weapon" name="x"><set name="bodypart" val="rhand"/><set name="enchant4_skill" val="notapair"/></item>`,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			writeItemFile(t, dir, "fixture.xml", c.content)

			log, hook := test.NewNullLogger()
			log.SetLevel(logrus.WarnLevel)

			table, err := LoadItemTemplates(dir, log)
			if err != nil {
				t.Fatalf("LoadItemTemplates: %v", err)
			}
			if table.Len() != 0 {
				t.Fatalf("table.Len() = %d, want 0", table.Len())
			}
			if len(hook.Entries) != 1 {
				t.Fatalf("warnings logged = %d, want 1", len(hook.Entries))
			}
		})
	}
}

// TestLoadItemTemplatesAgainstDatapack compares the loader's output against
// the shipped aCis_datapack item template files: the total count, and
// field-level values for a sample of items chosen to exercise weapon,
// armor and etc-item detail, stat modifiers, attached/triggered skills, and
// use conditions. Expected values were read directly off the data files
// themselves alongside the parsing rules above: every field asserted here
// is a plain attribute value or a simple, documented default/override rule,
// not a computed formula, so no external oracle run was needed to produce
// them.
func TestLoadItemTemplatesAgainstDatapack(t *testing.T) {
	dir := datapackPath(t, filepath.Join("data", "xml", "items"))

	log, hook := test.NewNullLogger()
	log.SetLevel(logrus.WarnLevel)

	table, err := LoadItemTemplates(dir, log)
	if err != nil {
		t.Fatalf("LoadItemTemplates(%q) error: %v", dir, err)
	}

	const wantTotal = 9208
	if got := table.Len(); got != wantTotal {
		t.Fatalf("table.Len() = %d, want %d", got, wantTotal)
	}
	if len(hook.Entries) != 0 {
		t.Fatalf("skipped-item warnings logged = %d, want 0: %v", len(hook.Entries), hook.Entries)
	}

	t.Run("Dagger (10): weapon fields and stat modifiers", func(t *testing.T) {
		tpl, ok := table.Get(10)
		if !ok {
			t.Fatal("item 10 not loaded")
		}
		if tpl.Name != "Dagger" || tpl.Kind != item.KindWeapon || tpl.Slot != item.SlotRHand {
			t.Fatalf("item 10 identity = %+v", tpl)
		}
		if tpl.Weight != 1160 || tpl.Material != item.MaterialSteel || tpl.ReferencePrice != 138 {
			t.Fatalf("item 10 base fields = %+v", tpl)
		}
		if tpl.Tradable || tpl.Dropable || tpl.Sellable {
			t.Fatalf("item 10 flags = %+v, want tradable/dropable/sellable all false", tpl)
		}
		if tpl.Weapon == nil || tpl.Weapon.Type != item.WeaponDagger {
			t.Fatalf("item 10 weapon type = %+v", tpl.Weapon)
		}
		if tpl.Weapon.SoulshotCount != 1 || tpl.Weapon.SpiritshotCount != 1 {
			t.Fatalf("item 10 shots = %+v", tpl.Weapon)
		}
		wantMods := []item.StatModifier{
			{Op: item.FuncSet, Stat: "pAtk", Value: 5},
			{Op: item.FuncSet, Stat: "mAtk", Value: 5},
			{Op: item.FuncSet, Stat: "rCrit", Value: 12},
			{Op: item.FuncSub, Stat: "accCombat", Value: 3},
			{Op: item.FuncSet, Stat: "pAtkSpd", Value: 433},
		}
		if len(tpl.Modifiers) != len(wantMods) {
			t.Fatalf("item 10 Modifiers = %+v, want %+v", tpl.Modifiers, wantMods)
		}
		for i, want := range wantMods {
			if tpl.Modifiers[i] != want {
				t.Fatalf("item 10 Modifiers[%d] = %+v, want %+v", i, tpl.Modifiers[i], want)
			}
		}
	})

	t.Run("Leather Shield (18): unset armor_type in the one-handed slot is a shield", func(t *testing.T) {
		tpl, ok := table.Get(18)
		if !ok {
			t.Fatal("item 18 not loaded")
		}
		if tpl.Name != "Leather Shield" || tpl.Slot != item.SlotLHand {
			t.Fatalf("item 18 identity = %+v", tpl)
		}
		if tpl.Armor == nil || tpl.Armor.Type != item.ArmorShield {
			t.Fatalf("item 18 armor detail = %+v, want ArmorShield", tpl.Armor)
		}
	})

	t.Run("Short Spear (15): item_skill attaches a passive skill", func(t *testing.T) {
		tpl, ok := table.Get(15)
		if !ok {
			t.Fatal("item 15 not loaded")
		}
		want := []item.SkillRef{{ID: 3599, Level: 1}}
		if len(tpl.AttachedSkills) != len(want) || tpl.AttachedSkills[0] != want[0] {
			t.Fatalf("item 15 AttachedSkills = %+v, want %+v", tpl.AttachedSkills, want)
		}
	})

	t.Run("Cursed Maingauche (1660): on-crit skill with an explicit chance", func(t *testing.T) {
		tpl, ok := table.Get(1660)
		if !ok {
			t.Fatal("item 1660 not loaded")
		}
		if tpl.Crystal != item.CrystalD || tpl.CrystalCount != 2545 {
			t.Fatalf("item 1660 crystal = %+v", tpl)
		}
		if tpl.Weapon == nil || tpl.Weapon.OnCritSkill == nil {
			t.Fatal("item 1660 OnCritSkill is nil")
		}
		want := item.SkillTrigger{Skill: item.SkillRef{ID: 3005, Level: 1}, Chance: 50}
		if *tpl.Weapon.OnCritSkill != want {
			t.Fatalf("item 1660 OnCritSkill = %+v, want %+v", tpl.Weapon.OnCritSkill, want)
		}
	})

	t.Run("Infinity Blade (6611): hero item with a use condition", func(t *testing.T) {
		tpl, ok := table.Get(6611)
		if !ok {
			t.Fatal("item 6611 not loaded")
		}
		if !tpl.HeroItem() {
			t.Fatal("item 6611 HeroItem() = false, want true")
		}
		if !tpl.OlyRestricted {
			t.Fatal("item 6611 OlyRestricted = false, want true")
		}
		if tpl.Tradable || tpl.Dropable || tpl.Destroyable || tpl.Sellable || tpl.Depositable {
			t.Fatalf("item 6611 flags = %+v, want all false", tpl)
		}
		if tpl.Weapon == nil || tpl.Weapon.OnCritSkill == nil {
			t.Fatal("item 6611 OnCritSkill is nil")
		}
		if tpl.Weapon.OnCritSkill.Chance != -1 {
			t.Fatalf("item 6611 OnCritSkill.Chance = %d, want -1 (unconditional)", tpl.Weapon.OnCritSkill.Chance)
		}
		if len(tpl.UseConditions) != 1 {
			t.Fatalf("item 6611 UseConditions = %+v, want 1 entry", tpl.UseConditions)
		}
		uc := tpl.UseConditions[0]
		if uc.MessageID != 1518 || uc.Root.Kind != "player" || uc.Root.Attrs["isHero"] != "true" {
			t.Fatalf("item 6611 UseConditions[0] = %+v", uc)
		}
	})

	t.Run("Soulshot: D-grade (1463): default action reclassifies it as a shot", func(t *testing.T) {
		tpl, ok := table.Get(1463)
		if !ok {
			t.Fatal("item 1463 not loaded")
		}
		if tpl.EtcItem == nil || tpl.EtcItem.Type != item.EtcItemShot {
			t.Fatalf("item 1463 etc item type = %+v, want EtcItemShot", tpl.EtcItem)
		}
		if tpl.EtcItem.Handler != "SoulShots" {
			t.Fatalf("item 1463 handler = %q, want SoulShots", tpl.EtcItem.Handler)
		}
	})

	t.Run("Lesser Healing Potion (1060): reuse group and delay", func(t *testing.T) {
		tpl, ok := table.Get(1060)
		if !ok {
			t.Fatal("item 1060 not loaded")
		}
		if tpl.EtcItem == nil || tpl.EtcItem.Type != item.EtcItemPotion {
			t.Fatalf("item 1060 etc item type = %+v, want EtcItemPotion", tpl.EtcItem)
		}
		if tpl.EtcItem.SharedReuseGroup != 8 || tpl.EtcItem.ReuseDelay != 10000 {
			t.Fatalf("item 1060 reuse fields = %+v", tpl.EtcItem)
		}
		if !tpl.OlyRestricted {
			t.Fatal("item 1060 OlyRestricted = false, want true")
		}
	})
}

// TestLoadItemTemplatesNoDuplicateIDsInDatapack cross-checks that the
// shipped item template files never define the same item id twice, a
// standing assumption of the loader's last-write-wins table construction:
// if any id appeared in more than one file, the table (deduplicated by id)
// would hold fewer entries than the raw count of <item> elements parsed.
func TestLoadItemTemplatesNoDuplicateIDsInDatapack(t *testing.T) {
	dir := datapackPath(t, filepath.Join("data", "xml", "items"))

	table, err := LoadItemTemplates(dir, nil)
	if err != nil {
		t.Fatalf("LoadItemTemplates(%q) error: %v", dir, err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	total := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		parsed, err := parseItemFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("parse %s: %v", e.Name(), err)
		}
		total += len(parsed.Items)
	}

	if total != table.Len() {
		t.Fatalf("parsed %d <item> elements across all files but table holds %d entries; some id is defined more than once", total, table.Len())
	}
}
