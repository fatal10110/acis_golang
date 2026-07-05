package xml

import (
	"bufio"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
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
		<for>
			<set stat="pAtk" val="5" />
		</for>
	</item>
	<item id="11" type="Armor" name="Squire's Shirt">
		<set name="armor_type" val="LIGHT" />
		<set name="bodypart" val="chest" />
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

	if got, want := table.Len(), 4; got != want {
		t.Fatalf("table.Len() = %d, want %d", got, want)
	}
	if got := len(hook.Entries); got != 3 {
		t.Fatalf("skipped-item warnings logged = %d, want 3", got)
	}

	cases := []struct {
		id        int32
		wantName  string
		wantKind  item.Kind
		wantSlot  item.Slot
		wantStack bool
		wantEquip bool
	}{
		{10, "Dagger", item.KindWeapon, item.SlotRHand, false, true},
		{11, "Squire's Shirt", item.KindArmor, item.SlotChest, false, true},
		{5588, "Tutorial Guide", item.KindEtcItem, item.SlotNone, false, false},
		{57, "Adena", item.KindEtcItem, item.SlotNone, true, false},
	}
	for _, c := range cases {
		tpl, ok := table.Get(c.id)
		if !ok {
			t.Errorf("Get(%d): not found", c.id)
			continue
		}
		if tpl.Name != c.wantName {
			t.Errorf("Get(%d).Name = %q, want %q", c.id, tpl.Name, c.wantName)
		}
		if tpl.Kind != c.wantKind {
			t.Errorf("Get(%d).Kind = %v, want %v", c.id, tpl.Kind, c.wantKind)
		}
		if tpl.Slot != c.wantSlot {
			t.Errorf("Get(%d).Slot = %v, want %v", c.id, tpl.Slot, c.wantSlot)
		}
		if tpl.Stackable != c.wantStack {
			t.Errorf("Get(%d).Stackable = %v, want %v", c.id, tpl.Stackable, c.wantStack)
		}
		if got := tpl.Equipable(); got != c.wantEquip {
			t.Errorf("Get(%d).Equipable() = %v, want %v", c.id, got, c.wantEquip)
		}
	}

	for _, badID := range []int32{99, 98} {
		if _, ok := table.Get(badID); ok {
			t.Errorf("Get(%d): expected the malformed entry to be skipped", badID)
		}
	}
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

// TestLoadItemTemplatesAgainstDatapack compares the loader's output against
// the shipped aCis_datapack item template files, for both the total count
// and known field values, established by running the reference item
// template parser against the same files and recording its output. It's
// skipped unless ACIS_DATAPACK_ITEMS_DIR points at that directory, since a
// generic checkout of this module doesn't carry the data files.
func TestLoadItemTemplatesAgainstDatapack(t *testing.T) {
	dir := os.Getenv("ACIS_DATAPACK_ITEMS_DIR")
	if dir == "" {
		t.Skip("set ACIS_DATAPACK_ITEMS_DIR to the aCis_datapack data/xml/items directory to run this test")
	}

	table, err := LoadItemTemplates(dir, nil)
	if err != nil {
		t.Fatalf("LoadItemTemplates: %v", err)
	}

	const wantTotal = 9208
	if got := table.Len(); got != wantTotal {
		t.Fatalf("table.Len() = %d, want %d", got, wantTotal)
	}

	// Starter-gear items granted at character creation (see the classes
	// templates' <items> lists) plus the base fist weapon. Field values
	// were recorded from the reference item template parser's output for
	// these ids against the same data files.
	cases := []struct {
		id        int32
		wantName  string
		wantKind  item.Kind
		wantSlot  item.Slot
		wantStack bool
	}{
		{10, "Dagger", item.KindWeapon, item.SlotRHand, false},
		{246, "Human Fighter Fist", item.KindWeapon, item.SlotRHand, false},
		{1146, "Squire's Shirt", item.KindArmor, item.SlotChest, false},
		{1147, "Squire's Pants", item.KindArmor, item.SlotLegs, false},
		{2369, "Squire's Sword", item.KindWeapon, item.SlotRHand, false},
		{5588, "Tutorial Guide", item.KindEtcItem, item.SlotNone, false},
	}
	for _, c := range cases {
		tpl, ok := table.Get(c.id)
		if !ok {
			t.Errorf("Get(%d): not found", c.id)
			continue
		}
		if tpl.Name != c.wantName || tpl.Kind != c.wantKind || tpl.Slot != c.wantSlot || tpl.Stackable != c.wantStack {
			t.Errorf("Get(%d) = %+v, want {Name:%q Kind:%v Slot:%v Stackable:%v}", c.id, tpl, c.wantName, c.wantKind, c.wantSlot, c.wantStack)
		}
	}
}

// TestLoadItemTemplatesNoDuplicateIDsInDatapack cross-checks that the
// shipped item template files never define the same item id twice, a
// standing assumption of the loader's last-write-wins table construction.
// Skipped under the same conditions as the oracle comparison above.
func TestLoadItemTemplatesNoDuplicateIDsInDatapack(t *testing.T) {
	dir := os.Getenv("ACIS_DATAPACK_ITEMS_DIR")
	if dir == "" {
		t.Skip("set ACIS_DATAPACK_ITEMS_DIR to the aCis_datapack data/xml/items directory to run this test")
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	seen := make(map[string]string) // id -> file it was first seen in
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".xml") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)

	for _, name := range files {
		f, err := os.Open(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := scanner.Text()
			idx := strings.Index(line, `<item id="`)
			if idx < 0 {
				continue
			}
			rest := line[idx+len(`<item id="`):]
			end := strings.Index(rest, `"`)
			if end < 0 {
				continue
			}
			id := rest[:end]
			if _, err := strconv.Atoi(id); err != nil {
				continue
			}
			if prev, ok := seen[id]; ok {
				t.Errorf("item id %s defined in both %s and %s", id, prev, name)
			} else {
				seen[id] = name
			}
		}
		f.Close()
	}
}
