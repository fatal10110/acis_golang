package cache

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadCrests(t *testing.T) {
	dir := t.TempDir()
	pledge := bytes.Repeat([]byte{0x11}, crestSize(PledgeCrest))
	large := bytes.Repeat([]byte{0x22}, crestSize(LargePledgeCrest))
	ally := bytes.Repeat([]byte{0x33}, crestSize(AllyCrest))

	writeCrestFixture(t, dir, "Crest_101.dds", pledge)
	writeCrestFixture(t, dir, "LargeCrest_102.dds", large)
	writeCrestFixture(t, dir, "AllyCrest_103.dds", ally)
	writeCrestFixture(t, dir, "OtherCrest_104.dds", []byte{0x44})
	writeCrestFixture(t, dir, "Crest_105.png", bytes.Repeat([]byte{0x55}, crestSize(PledgeCrest)))

	crests, _, err := LoadCrests(dir)
	if err != nil {
		t.Fatalf("LoadCrests(%q) error: %v", dir, err)
	}
	if got := crests.Len(); got != 3 {
		t.Fatalf("Len() = %d, want 3", got)
	}

	assertCrest(t, crests, PledgeCrest, 101, pledge)
	assertCrest(t, crests, LargePledgeCrest, 102, large)
	assertCrest(t, crests, AllyCrest, 103, ally)

	if _, ok := crests.Get(PledgeCrest, 102); ok {
		t.Fatal("Get(PledgeCrest, 102) returned large crest data, want missing")
	}
	if _, ok := crests.Get(PledgeCrest, 999); ok {
		t.Fatal("Get(PledgeCrest, 999) returned data, want missing")
	}
}

func TestLoadCrestsCopiesReturnedData(t *testing.T) {
	dir := t.TempDir()
	want := bytes.Repeat([]byte{0x11}, crestSize(PledgeCrest))
	writeCrestFixture(t, dir, "Crest_101.dds", want)

	crests, _, err := LoadCrests(dir)
	if err != nil {
		t.Fatalf("LoadCrests(%q) error: %v", dir, err)
	}

	got, ok := crests.Get(PledgeCrest, 101)
	if !ok {
		t.Fatal("Get(PledgeCrest, 101) missing, want present")
	}
	got[0] = 0xff

	assertCrest(t, crests, PledgeCrest, 101, want)
}

func TestLoadCrestsErrors(t *testing.T) {
	t.Run("missing directory", func(t *testing.T) {
		if c, _, err := LoadCrests(filepath.Join(t.TempDir(), "missing")); err == nil || c != nil {
			t.Fatalf("LoadCrests(missing) = %v, %v; want no cache and an error", c, err)
		}
	})

	t.Run("wrong size is deleted and skipped", func(t *testing.T) {
		dir := t.TempDir()
		valid := bytes.Repeat([]byte{0x11}, crestSize(PledgeCrest))
		invalidPath := filepath.Join(dir, "Crest_102.dds")
		writeCrestFixture(t, dir, "Crest_101.dds", valid)
		writeCrestFixture(t, dir, "Crest_102.dds", []byte{0x11})

		crests, deleted, err := LoadCrests(dir)
		if err != nil {
			t.Fatalf("LoadCrests(%q) error: %v", dir, err)
		}
		assertCrest(t, crests, PledgeCrest, 101, valid)
		if _, err := os.Stat(invalidPath); !os.IsNotExist(err) {
			t.Fatalf("invalid crest still exists: %v", err)
		}
		if len(deleted) != 1 || deleted[0] != "Crest_102.dds" {
			t.Fatalf("deleted = %v, want [Crest_102.dds]", deleted)
		}
	})

	t.Run("bad id stops the load, keeps the partial cache and reports why", func(t *testing.T) {
		dir := t.TempDir()
		valid := bytes.Repeat([]byte{0x11}, crestSize(PledgeCrest))
		writeCrestFixture(t, dir, "Crest_101.dds", valid)
		writeCrestFixture(t, dir, "Crest_bad.dds", bytes.Repeat([]byte{0x11}, crestSize(PledgeCrest)))
		// Read after Crest_bad.dds in directory order, so never loaded.
		writeCrestFixture(t, dir, "LargeCrest_103.dds", bytes.Repeat([]byte{0x22}, crestSize(LargePledgeCrest)))

		crests, _, err := LoadCrests(dir)
		if err == nil || !strings.Contains(err.Error(), "Crest_bad.dds") {
			t.Fatalf("LoadCrests(%q) error = %v, want one naming Crest_bad.dds", dir, err)
		}
		if crests == nil {
			t.Fatal("LoadCrests returned no cache, want the partial cache")
		}
		assertCrest(t, crests, PledgeCrest, 101, valid)
		if crests.Has(LargePledgeCrest, 103) {
			t.Fatal("LargeCrest_103 loaded after the bad file, want the load stopped there")
		}
		if crests.Len() != 1 {
			t.Fatalf("Len() = %d, want 1", crests.Len())
		}
	})
}

func assertCrest(t *testing.T, crests *Crests, typ CrestType, id int, want []byte) {
	t.Helper()
	got, ok := crests.Get(typ, id)
	if !ok {
		t.Fatalf("Get(%+v, %d) missing, want present", typ, id)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("Get(%+v, %d) bytes changed", typ, id)
	}
}

func writeCrestFixture(t *testing.T, dir, name string, data []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
		t.Fatalf("write fixture %s: %v", name, err)
	}
}

func crestSize(typ CrestType) int {
	spec, ok := typ.spec()
	if !ok {
		panic("bad test crest type")
	}
	return spec.size
}
