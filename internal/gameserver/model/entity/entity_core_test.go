package entity

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// ---- from cursedweapon_test.go ----
func TestNewCursedWeapon(t *testing.T) {
	weapon, err := NewCursedWeapon(8190, 3603, "Demonic Sword Zariche", 1, 72, 24, 50, 10, skill.NewTable([]skill.Definition{{ID: 3603, Level: 13}}))
	if err != nil {
		t.Fatalf("NewCursedWeapon() error: %v", err)
	}
	if weapon.ItemID != 8190 || weapon.Skill.ID != 3603 || weapon.Skill.Level != 13 {
		t.Fatalf("NewCursedWeapon() = %+v", weapon)
	}
}

func TestNewCursedWeaponTableRejectsDuplicateIDs(t *testing.T) {
	_, err := NewCursedWeaponTable([]CursedWeapon{{ItemID: 8190}, {ItemID: 8190}})
	if err == nil {
		t.Fatal("expected duplicate item id error, got nil")
	}
}

func TestCursedWeaponTableIDsAreSorted(t *testing.T) {
	table, err := NewCursedWeaponTable([]CursedWeapon{{ItemID: 8689}, {ItemID: 8190}})
	if err != nil {
		t.Fatalf("NewCursedWeaponTable() error: %v", err)
	}
	got := table.IDs()
	want := []int32{8190, 8689}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("IDs() = %v, want %v", got, want)
	}

	got[0] = 1
	again := table.IDs()
	if again[0] != want[0] {
		t.Fatalf("IDs() returned mutable backing slice: %v", again)
	}
}
