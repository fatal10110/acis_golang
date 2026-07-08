package entity

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

func TestNewCursedWeapon(t *testing.T) {
	set := commons.NewStatSet()
	set.Set("id", "8190")
	set.Set("skillId", "3603")
	set.Set("name", "Demonic Sword Zariche")
	set.Set("dropRate", "1")
	set.Set("duration", "72")
	set.Set("durationLost", "24")
	set.Set("dissapearChance", "50")
	set.Set("stageKills", "10")

	weapon, err := NewCursedWeapon(set, skill.NewTable([]skill.Definition{{ID: 3603, Level: 13}}))
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
