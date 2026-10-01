package cache

import (
	"bytes"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// TestCrestsSameIDDifferentFamily keeps a pledge, large pledge and alliance
// crest sharing one id apart: each loads and is served on its own.
func TestCrestsSameIDDifferentFamily(t *testing.T) {
	dir := t.TempDir()
	pledge := bytes.Repeat([]byte{0x11}, crestSize(PledgeCrest))
	large := bytes.Repeat([]byte{0x22}, crestSize(LargePledgeCrest))
	ally := bytes.Repeat([]byte{0x33}, crestSize(AllyCrest))
	writeCrestFixture(t, dir, "Crest_5.dds", pledge)
	writeCrestFixture(t, dir, "LargeCrest_5.dds", large)
	writeCrestFixture(t, dir, "AllyCrest_5.dds", ally)

	crests, _, err := LoadCrests(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := crests.Len(); got != 3 {
		t.Fatalf("Len() = %d, want 3", got)
	}
	assertCrest(t, crests, PledgeCrest, 5, pledge)
	assertCrest(t, crests, LargePledgeCrest, 5, large)
	assertCrest(t, crests, AllyCrest, 5, ally)
}

// TestCrestsSaveAndRemove saves a crest to memory and to its family's file,
// refuses an image of the wrong size, and removes the one family's crest
// alone.
func TestCrestsSaveAndRemove(t *testing.T) {
	dir := t.TempDir()
	crests := NewCrestsIn(dir)
	pledge := bytes.Repeat([]byte{0x44}, crestSize(PledgeCrest))
	ally := bytes.Repeat([]byte{0x55}, crestSize(AllyCrest))

	if err := crests.Save(PledgeCrest, 9, pledge); err != nil {
		t.Fatalf("Save(pledge): %v", err)
	}
	if err := crests.Save(AllyCrest, 9, ally); err != nil {
		t.Fatalf("Save(ally): %v", err)
	}
	pledge[0] = 0 // the cache keeps its own copy
	assertCrest(t, crests, PledgeCrest, 9, bytes.Repeat([]byte{0x44}, crestSize(PledgeCrest)))
	if got, err := os.ReadFile(filepath.Join(dir, "Crest_9.dds")); err != nil || len(got) != crestSize(PledgeCrest) || got[0] != 0x44 {
		t.Fatalf("Crest_9.dds = %d bytes, %v", len(got), err)
	}
	if err := crests.Save(LargePledgeCrest, 10, pledge); err == nil {
		t.Fatal("Save accepted a pledge-sized large crest")
	}
	if crests.Has(LargePledgeCrest, 10) {
		t.Fatal("a refused large crest is cached")
	}
	if _, err := os.Stat(filepath.Join(dir, "LargeCrest_10.dds")); !os.IsNotExist(err) {
		t.Fatalf("a refused large crest was written: %v", err)
	}

	if err := crests.Remove(PledgeCrest, 9); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if crests.Has(PledgeCrest, 9) || !crests.Has(AllyCrest, 9) {
		t.Fatal("Remove(pledge 9) did not remove only the pledge crest")
	}
	if _, err := os.Stat(filepath.Join(dir, "Crest_9.dds")); !os.IsNotExist(err) {
		t.Fatalf("Crest_9.dds still exists: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "AllyCrest_9.dds")); err != nil {
		t.Fatalf("AllyCrest_9.dds: %v", err)
	}

	if err := NewCrests().Save(PledgeCrest, 1, bytes.Repeat([]byte{1}, crestSize(PledgeCrest))); err == nil {
		t.Fatal("a cache with no directory saved a crest")
	}
}

// TestCrestsConcurrentSaveAndGet saves crests while others are read; run
// under -race.
func TestCrestsConcurrentSaveAndGet(t *testing.T) {
	crests := NewCrestsIn(t.TempDir())
	data := bytes.Repeat([]byte{0x66}, crestSize(PledgeCrest))
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			if err := crests.Save(PledgeCrest, i, data); err != nil {
				t.Errorf("Save(%d): %v", i, err)
			}
		}()
		go func() {
			defer wg.Done()
			crests.Get(PledgeCrest, i)
			crests.Len()
		}()
	}
	wg.Wait()
	if got := crests.Len(); got != 8 {
		t.Fatalf("Len() = %d, want 8", got)
	}
}
