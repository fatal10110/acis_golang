package cache

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// A crest directory holding valid crests of both kinds, a wrong-size file of
// each kind, a bad id at the right size and a stray name loads every valid
// crest, deletes exactly the wrong-size files, keeps the others on disk and
// reports each unusable .dds name once and ignores files that are not .dds; no clan can point at those names, so the
// cache is not partial.
func TestLoadCrestsMixedValidInvalidSizeAndBadlyNamed(t *testing.T) {
	dir := t.TempDir()
	pledge := bytes.Repeat([]byte{0x11}, crestSize(PledgeCrest))
	large := bytes.Repeat([]byte{0x22}, crestSize(LargePledgeCrest))
	writeCrestFixture(t, dir, "Crest_100.dds", pledge)
	writeCrestFixture(t, dir, "Crest_101.dds", pledge[:len(pledge)-1])
	writeCrestFixture(t, dir, "Crest_zzz.dds", pledge)
	writeCrestFixture(t, dir, "Crest_200.dds", pledge)
	writeCrestFixture(t, dir, "LargeCrest_300.dds", large)
	writeCrestFixture(t, dir, "LargeCrest_301.dds", append(large, 0x33))
	writeCrestFixture(t, dir, "LargeCrest_x.dds", large)
	writeCrestFixture(t, dir, "Crest_150 - Copy.dds", pledge)
	writeCrestFixture(t, dir, "notes.txt", []byte("not a crest"))

	crests, deleted, err := LoadCrests(dir)
	if err == nil {
		t.Fatal("LoadCrests error = nil, want the bad names reported")
	}
	for _, name := range []string{"Crest_zzz.dds", "LargeCrest_x.dds", "Crest_150 - Copy.dds"} {
		if strings.Count(err.Error(), name) != 1 {
			t.Fatalf("LoadCrests error = %v, want %s named once", err, name)
		}
	}
	for _, name := range []string{"Crest_101.dds", "LargeCrest_301.dds", "notes.txt"} {
		if strings.Contains(err.Error(), name) {
			t.Fatalf("LoadCrests error = %v, names %s", err, name)
		}
	}
	slices.Sort(deleted)
	if want := []string{"Crest_101.dds", "LargeCrest_301.dds"}; !slices.Equal(deleted, want) {
		t.Fatalf("deleted = %v, want %v", deleted, want)
	}
	for _, name := range []string{"Crest_101.dds", "LargeCrest_301.dds"} {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Fatalf("%s still on disk: %v", name, err)
		}
	}
	for _, name := range []string{"Crest_zzz.dds", "LargeCrest_x.dds", "Crest_150 - Copy.dds", "notes.txt"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("%s was touched: %v", name, err)
		}
	}
	assertCrest(t, crests, PledgeCrest, 100, pledge)
	assertCrest(t, crests, PledgeCrest, 200, pledge)
	assertCrest(t, crests, LargePledgeCrest, 300, large)
	if crests.Len() != 3 || crests.Partial() {
		t.Fatalf("Len() = %d, Partial() = %v; want 3 crests, not partial", crests.Len(), crests.Partial())
	}
}
