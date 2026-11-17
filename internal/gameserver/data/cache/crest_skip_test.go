package cache

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A stray file between valid crests is skipped, not a stop: every valid
// crest after it in directory order still loads, and the cache is not
// partial since no clan crest id can name the stray file.
func TestLoadCrestsSkipsStrayFile(t *testing.T) {
	dir := t.TempDir()
	pledge := bytes.Repeat([]byte{0x11}, crestSize(PledgeCrest))
	large := bytes.Repeat([]byte{0x22}, crestSize(LargePledgeCrest))
	writeCrestFixture(t, dir, "Crest_100.dds", pledge)
	writeCrestFixture(t, dir, "Crest_123 - Copy.dds", pledge)
	writeCrestFixture(t, dir, "Crest_200.dds", pledge)
	writeCrestFixture(t, dir, "LargeCrest_300.dds", large)

	crests, deleted, err := LoadCrests(dir)
	if err == nil || !strings.Contains(err.Error(), "Crest_123 - Copy.dds") {
		t.Fatalf("LoadCrests error = %v, want one naming the stray file", err)
	}
	if len(deleted) != 0 {
		t.Fatalf("deleted = %v, want none", deleted)
	}
	assertCrest(t, crests, PledgeCrest, 100, pledge)
	assertCrest(t, crests, PledgeCrest, 200, pledge)
	assertCrest(t, crests, LargePledgeCrest, 300, large)
	if crests.Len() != 3 || crests.Partial() {
		t.Fatalf("Len() = %d, Partial() = %v; want 3 crests, not partial", crests.Len(), crests.Partial())
	}
	if _, err := os.Stat(filepath.Join(dir, "Crest_123 - Copy.dds")); err != nil {
		t.Fatalf("the stray file was touched: %v", err)
	}
}

// The size is checked before the id: a wrong-size file whose id is not a
// number is deleted like any invalid crest, and the load goes on.
func TestLoadCrestsDeletesWrongSizeBeforeParsingID(t *testing.T) {
	dir := t.TempDir()
	pledge := bytes.Repeat([]byte{0x11}, crestSize(PledgeCrest))
	large := bytes.Repeat([]byte{0x22}, crestSize(LargePledgeCrest))
	writeCrestFixture(t, dir, "Crest_101.dds", pledge)
	writeCrestFixture(t, dir, "Crest_abc.dds", []byte{0x01})
	writeCrestFixture(t, dir, "Crest_zzz.dds", pledge)
	writeCrestFixture(t, dir, "LargeCrest_7.dds", large)

	crests, deleted, err := LoadCrests(dir)
	if err == nil || !strings.Contains(err.Error(), "Crest_zzz.dds") || strings.Contains(err.Error(), "Crest_abc.dds") {
		t.Fatalf("LoadCrests error = %v, want one naming Crest_zzz.dds only", err)
	}
	if len(deleted) != 1 || deleted[0] != "Crest_abc.dds" {
		t.Fatalf("deleted = %v, want [Crest_abc.dds]", deleted)
	}
	if _, err := os.Stat(filepath.Join(dir, "Crest_abc.dds")); !os.IsNotExist(err) {
		t.Fatalf("Crest_abc.dds still exists: %v", err)
	}
	assertCrest(t, crests, PledgeCrest, 101, pledge)
	assertCrest(t, crests, LargePledgeCrest, 7, large)
	if crests.Len() != 2 {
		t.Fatalf("Len() = %d, want 2", crests.Len())
	}
}

// An unreadable crest file is skipped and the rest loads, but the cache is
// partial: that file may be a crest a clan points at.
func TestLoadCrestsUnreadableFileIsPartial(t *testing.T) {
	dir := t.TempDir()
	pledge := bytes.Repeat([]byte{0x11}, crestSize(PledgeCrest))
	large := bytes.Repeat([]byte{0x22}, crestSize(LargePledgeCrest))
	writeCrestFixture(t, dir, "Crest_100.dds", pledge)
	if err := os.Symlink(filepath.Join(dir, "gone"), filepath.Join(dir, "Crest_150.dds")); err != nil {
		t.Fatal(err)
	}
	writeCrestFixture(t, dir, "LargeCrest_300.dds", large)

	crests, _, err := LoadCrests(dir)
	if err == nil || !strings.Contains(err.Error(), "Crest_150.dds") {
		t.Fatalf("LoadCrests error = %v, want one naming Crest_150.dds", err)
	}
	assertCrest(t, crests, PledgeCrest, 100, pledge)
	assertCrest(t, crests, LargePledgeCrest, 300, large)
	if crests.Len() != 2 || !crests.Partial() {
		t.Fatalf("Len() = %d, Partial() = %v; want 2 crests, partial", crests.Len(), crests.Partial())
	}
}
