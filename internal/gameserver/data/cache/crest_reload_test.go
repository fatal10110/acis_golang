package cache

import (
	"bytes"
	"errors"
	"testing"
)

// A crest saved or removed while a reload reads the directory keeps that
// change through the swap: the save is served, the removed crest is not.
func TestCrestsReloadKeepsWritesMadeDuringTheRead(t *testing.T) {
	dir := t.TempDir()
	pledge := bytes.Repeat([]byte{0x11}, crestSize(PledgeCrest))
	saved := bytes.Repeat([]byte{0x33}, crestSize(PledgeCrest))
	writeCrestFixture(t, dir, "Crest_1.dds", pledge)
	writeCrestFixture(t, dir, "Crest_2.dds", pledge)
	crests, _, err := LoadCrests(dir)
	if err != nil {
		t.Fatal(err)
	}

	err = crests.Reload(func(dir string) (*Crests, error) {
		fresh, _, err := LoadCrests(dir)
		if err != nil {
			return nil, err
		}
		// The read is done; these land before the swap.
		if err := crests.Save(PledgeCrest, 3, saved); err != nil {
			t.Errorf("Save during the reload: %v", err)
		}
		if err := crests.Remove(PledgeCrest, 1); err != nil {
			t.Errorf("Remove during the reload: %v", err)
		}
		return fresh, nil
	})
	if err != nil {
		t.Fatalf("Reload: %v", err)
	}

	assertCrest(t, crests, PledgeCrest, 3, saved)
	assertCrest(t, crests, PledgeCrest, 2, pledge)
	if crests.Has(PledgeCrest, 1) {
		t.Fatal("a crest removed during the reload is served again")
	}

	// The next reload no longer tracks writes: it serves the directory.
	if err := crests.Reload(func(dir string) (*Crests, error) {
		fresh, _, err := LoadCrests(dir)
		return fresh, err
	}); err != nil {
		t.Fatalf("second Reload: %v", err)
	}
	assertCrest(t, crests, PledgeCrest, 3, saved)
	if crests.Has(PledgeCrest, 1) || crests.Len() != 2 {
		t.Fatalf("after the second reload Len() = %d, want Crest_2 and Crest_3", crests.Len())
	}
}

// A failed reload leaves the cache as it was, and the writes it saw stay.
func TestCrestsReloadFailureKeepsCache(t *testing.T) {
	dir := t.TempDir()
	pledge := bytes.Repeat([]byte{0x11}, crestSize(PledgeCrest))
	writeCrestFixture(t, dir, "Crest_1.dds", pledge)
	crests, _, err := LoadCrests(dir)
	if err != nil {
		t.Fatal(err)
	}
	failure := errors.New("unreadable")
	err = crests.Reload(func(string) (*Crests, error) {
		if err := crests.Save(PledgeCrest, 2, pledge); err != nil {
			t.Errorf("Save during the reload: %v", err)
		}
		return nil, failure
	})
	if !errors.Is(err, failure) {
		t.Fatalf("Reload = %v, want %v", err, failure)
	}
	assertCrest(t, crests, PledgeCrest, 1, pledge)
	assertCrest(t, crests, PledgeCrest, 2, pledge)
}
