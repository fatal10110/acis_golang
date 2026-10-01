package admin

import "testing"

// TestAccessLevelColors pins the access-level colors as the reference
// decodes them ("0x" + the attribute) and the fallback of a level built
// without them.
func TestAccessLevelColors(t *testing.T) {
	name, title := AccessLevel{NameColor: "CC6600", TitleColor: "00CCFF"}.Colors()
	if name != 0xCC6600 || title != 0x00CCFF {
		t.Fatalf("Colors() = %#x %#x, want 0xcc6600 0xccff", name, title)
	}
	name, title = AccessLevel{}.Colors()
	if name != 0xFFFFFF || title != 0xFFFF77 {
		t.Fatalf("unset Colors() = %#x %#x, want the defaults", name, title)
	}
}

// TestMasterAndDefinedLevels pins the master level as the highest defined
// one and every negative level reading the -1 entry.
func TestMasterAndDefinedLevels(t *testing.T) {
	d, err := NewData([]AccessLevel{{Level: -1}, {Level: 0}, {Level: 8}, {Level: 3}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := d.MasterLevel(); got != 8 {
		t.Fatalf("MasterLevel() = %d, want 8", got)
	}
	for level, want := range map[int]bool{-100: true, -1: true, 0: true, 3: true, 5: false, 99: false} {
		if got := d.DefinesLevel(level); got != want {
			t.Fatalf("DefinesLevel(%d) = %v, want %v", level, got, want)
		}
	}
	var none *Data
	if none.MasterLevel() != 0 || none.DefinesLevel(0) {
		t.Fatal("nil table defines a level")
	}
}
