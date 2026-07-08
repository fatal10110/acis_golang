package xml

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons"
)

func writeMultiSellFile(t *testing.T, dir, name, body string) {
	t.Helper()
	content := "<?xml version=\"1.0\" encoding=\"utf-8\"?>\n"
	if strings.Contains(body, "<list") {
		content += body + "\n"
	} else {
		content += "<list>\n" + body + "\n</list>\n"
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLoadMultiSellLists(t *testing.T) {
	itemDir := t.TempDir()
	writeItemFile(t, itemDir, "items.xml", `
	<item id="57" type="EtcItem" name="Adena">
		<set name="is_stackable" val="true" />
		<set name="weight" val="1" />
	</item>
	<item id="1000" type="Weapon" name="Sword">
		<set name="bodypart" val="rhand" />
		<set name="weight" val="1600" />
	</item>`)
	items, err := LoadItemTemplates(itemDir)
	if err != nil {
		t.Fatalf("LoadItemTemplates(%q): %v", itemDir, err)
	}

	dir := t.TempDir()
	writeMultiSellFile(t, dir, "1005.xml", `
<list maintainEnchantment="true" applyTaxes="true">
	<npcs>
		<npc>30846</npc>
		<npc>30847</npc>
	</npcs>
	<item>
		<production id="1000" count="1"/>
		<ingredient id="57" count="5000" isTaxIngredient="true"/>
	</item>
	<item>
		<production id="57" count="10"/>
		<ingredient id="1000" count="1" maintainIngredient="true" enchantLevel="4"/>
	</item>
</list>`)

	table, err := LoadMultiSellLists(dir, items)
	if err != nil {
		t.Fatalf("LoadMultiSellLists(%q): %v", dir, err)
	}
	if got, want := table.Count(), 1; got != want {
		t.Fatalf("table.Count() = %d, want %d", got, want)
	}

	id := commons.LegacyStringHash("1005")
	list, ok := table.Get(id)
	if !ok {
		t.Fatalf("list %d not loaded", id)
	}
	if !list.ApplyTaxes || !list.MaintainEnchantment {
		t.Fatalf("list flags = applyTaxes=%v maintainEnchantment=%v, want both true", list.ApplyTaxes, list.MaintainEnchantment)
	}
	if got, want := list.NPCIDs, []int32{30846, 30847}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("list.NPCIDs = %v, want %v", got, want)
	}
	if !list.NPCAllowed(30846) || list.NPCAllowed(99999) || !list.NPCOnly() {
		t.Fatalf("npc restrictions not preserved: %+v", list)
	}
	if got, want := len(list.Entries), 2; got != want {
		t.Fatalf("len(list.Entries) = %d, want %d", got, want)
	}

	first := list.Entries[0]
	if first.Stackable() {
		t.Fatalf("first entry Stackable() = true, want false for a weapon product")
	}
	if got, want := first.Products[0].Weight(), int32(1600); got != want {
		t.Fatalf("first product Weight() = %d, want %d", got, want)
	}
	if !first.Products[0].ArmorOrWeapon() {
		t.Fatal("first product ArmorOrWeapon() = false, want true")
	}
	if !first.Ingredients[0].TaxIngredient || first.Ingredients[0].MaintainIngredient {
		t.Fatalf("first ingredient flags = %+v", first.Ingredients[0])
	}

	second := list.Entries[1]
	if !second.Stackable() {
		t.Fatalf("second entry Stackable() = false, want true for adena product")
	}
	if got, want := second.Ingredients[0].EnchantLevel, 4; got != want {
		t.Fatalf("second ingredient EnchantLevel = %d, want %d", got, want)
	}
	if !second.Ingredients[0].MaintainIngredient {
		t.Fatalf("second ingredient MaintainIngredient = false, want true")
	}
}

func TestLoadMultiSellListsFilenameKeying(t *testing.T) {
	dir := t.TempDir()
	writeMultiSellFile(t, dir, "1.xml", `
	<item>
		<production id="57" count="1"/>
	</item>`)

	table, err := LoadMultiSellLists(dir, nil)
	if err != nil {
		t.Fatalf("LoadMultiSellLists(%q): %v", dir, err)
	}

	want := commons.LegacyStringHash("1")
	if _, ok := table.Get(want); !ok {
		t.Fatalf("list keyed by %d (hash of bare filename) not found", want)
	}
	if _, ok := table.Get(commons.LegacyStringHash(filepath.Join(dir, "1.xml"))); ok {
		t.Fatal("list was keyed by path, want bare filename only")
	}
}

func TestLoadMultiSellListsErrors(t *testing.T) {
	itemDir := t.TempDir()
	writeItemFile(t, itemDir, "items.xml", `
	<item id="57" type="EtcItem" name="Adena">
		<set name="is_stackable" val="true" />
	</item>`)
	items, err := LoadItemTemplates(itemDir)
	if err != nil {
		t.Fatalf("LoadItemTemplates(%q): %v", itemDir, err)
	}

	cases := []struct {
		name    string
		content string
	}{
		{
			name:    "malformed xml",
			content: `<item><production id="57" count="1"></list>`,
		},
		{
			name: "ingredient missing count",
			content: `
				<item>
					<production id="57" count="1"/>
					<ingredient id="57"/>
				</item>`,
		},
		{
			name: "production count not an integer",
			content: `
				<item>
					<production id="57" count="x"/>
				</item>`,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			writeMultiSellFile(t, dir, "fixture.xml", c.content)
			_, err := LoadMultiSellLists(dir, items)
			if err == nil {
				t.Fatalf("expected an error for %s, got nil", c.name)
			}
			if !strings.Contains(err.Error(), "fixture.xml") {
				t.Fatalf("error %q does not mention fixture.xml", err)
			}
		})
	}

	t.Run("empty directory", func(t *testing.T) {
		empty := t.TempDir()
		if _, err := LoadMultiSellLists(empty, items); err == nil {
			t.Fatal("expected an error for an empty directory, got nil")
		}
	})

	t.Run("missing directory", func(t *testing.T) {
		if _, err := LoadMultiSellLists(filepath.Join(t.TempDir(), "does-not-exist"), items); err == nil {
			t.Fatal("expected an error for a missing directory, got nil")
		}
	})
}

func TestLoadMultiSellListsDatapackSmoke(t *testing.T) {
	itemsDir := datapackPath(t, filepath.Join("data", "xml", "items"))
	items, err := LoadItemTemplates(itemsDir)
	if err != nil {
		t.Fatalf("LoadItemTemplates(%q): %v", itemsDir, err)
	}

	dir := datapackPath(t, filepath.Join("data", "xml", "multisell"))
	table, err := LoadMultiSellLists(dir, items)
	if err != nil {
		t.Fatalf("LoadMultiSellLists(%q): %v", dir, err)
	}

	if got, want := table.Count(), 85; got != want {
		t.Fatalf("table.Count() = %d, want %d", got, want)
	}

	id := commons.LegacyStringHash("1000")
	list, ok := table.Get(id)
	if !ok {
		t.Fatalf("list 1000 (id %d) not loaded", id)
	}
	if list.ApplyTaxes || list.MaintainEnchantment {
		t.Fatalf("list 1000 flags = applyTaxes=%v maintainEnchantment=%v, want both false", list.ApplyTaxes, list.MaintainEnchantment)
	}
	if got, want := len(list.NPCIDs), 11; got != want {
		t.Fatalf("len(list 1000 NPCIDs) = %d, want %d", got, want)
	}
	if got, want := len(list.Entries), 54; got != want {
		t.Fatalf("len(list 1000 Entries) = %d, want %d", got, want)
	}

	first := list.Entries[0]
	if got, want := first.Products[0].ItemID, int32(3439); got != want {
		t.Fatalf("list 1000 first product item id = %d, want %d", got, want)
	}
	if got, want := first.Products[0].Count, 1; got != want {
		t.Fatalf("list 1000 first product count = %d, want %d", got, want)
	}
	if got, want := len(first.Ingredients), 3; got != want {
		t.Fatalf("list 1000 first ingredient count = %d, want %d", got, want)
	}
	if got, want := first.Ingredients[0].ItemID, int32(2505); got != want {
		t.Fatalf("list 1000 first ingredient item id = %d, want %d", got, want)
	}
}
