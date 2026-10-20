package xml

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/rs/zerolog"
)

// The table lists the makers file by file in file name order, and in
// declaration order inside each file, not by maker name.
func TestLoadSpawnlistKeepsMakerDeclarationOrder(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeXMLFixture(t, filepath.Join(dir, "20_20.xml"), `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="late" minZ="0" maxZ="10"><node x="0" y="0"/><node x="1" y="0"/><node x="1" y="1"/></territory>
	<npcmaker name="aa_late" territory="late" maximumNpcs="1"/>
</list>`)
	writeXMLFixture(t, filepath.Join(dir, "19_20.xml"), `<?xml version="1.0" encoding="utf-8"?>
<list>
	<territory name="early" minZ="0" maxZ="10"><node x="0" y="0"/><node x="1" y="0"/><node x="1" y="1"/></territory>
	<npcmaker name="zz_first" territory="early" maximumNpcs="1"/>
	<npcmaker name="Mm_second" territory="early" maximumNpcs="1"/>
	<npcmaker name="bb_third" territory="early" maximumNpcs="1"/>
</list>`)
	table, err := LoadSpawnlist(dir, zerolog.Nop(), 1)
	if err != nil {
		t.Fatalf("LoadSpawnlist: %v", err)
	}
	var got []string
	for _, m := range table.Makers() {
		got = append(got, m.Name)
	}
	if want := []string{"zz_first", "Mm_second", "bb_third", "aa_late"}; !slices.Equal(got, want) {
		t.Fatalf("Makers() = %q, want %q", got, want)
	}
}
