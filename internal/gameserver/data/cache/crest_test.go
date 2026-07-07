package cache

import (
	"bytes"
	"os"
	"path/filepath"
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

	crests, err := LoadCrests(dir)
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

	crests, err := LoadCrests(dir)
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
		if _, err := LoadCrests(filepath.Join(t.TempDir(), "missing")); err == nil {
			t.Fatal("expected an error for a missing crest directory, got nil")
		}
	})

	t.Run("wrong size", func(t *testing.T) {
		dir := t.TempDir()
		writeCrestFixture(t, dir, "Crest_101.dds", []byte{0x11})
		if _, err := LoadCrests(dir); err == nil {
			t.Fatal("expected an error for a wrong-size crest, got nil")
		}
	})

	t.Run("bad id", func(t *testing.T) {
		dir := t.TempDir()
		writeCrestFixture(t, dir, "Crest_bad.dds", bytes.Repeat([]byte{0x11}, crestSize(PledgeCrest)))
		if _, err := LoadCrests(dir); err == nil {
			t.Fatal("expected an error for a malformed crest id, got nil")
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
