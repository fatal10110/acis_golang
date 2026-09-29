package player

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// TestInventoryLimitFollowsConfigAndStat pins the player slot limit to the
// reference formula: the configured base for the race plus the
// inventoryLimit stat, truncated, with the shipped 80/100 when nothing is
// configured and an explicitly configured 0 kept as 0.
func TestInventoryLimitFollowsConfigAndStat(t *testing.T) {
	tests := []struct {
		name  string
		race  Race
		slots InventorySlots
		add   float64
		want  int
	}{
		{"unconfigured human", RaceHuman, InventorySlots{}, 0, 80},
		{"unconfigured dwarf", RaceDwarf, InventorySlots{}, 0, 100},
		{"configured human", RaceElf, InventorySlots{NoDwarf: 50, Dwarf: 70, Configured: true}, 0, 50},
		{"configured dwarf", RaceDwarf, InventorySlots{NoDwarf: 50, Dwarf: 70, Configured: true}, 0, 70},
		{"configured zero", RaceHuman, InventorySlots{NoDwarf: 0, Dwarf: 70, Configured: true}, 1, 1},
		{"stat adds", RaceHuman, InventorySlots{NoDwarf: 50, Dwarf: 70, Configured: true}, 1, 51},
		{"stat truncates", RaceDwarf, InventorySlots{NoDwarf: 50, Dwarf: 70, Configured: true}, 2.9, 72},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := &Character{Race: tc.race}
			c.Configure(Runtime{Rules: Rules{InventorySlots: tc.slots}})
			if tc.add != 0 {
				c.AddStatFuncs([]effect.Mod{{Stat: stat.InvLim, Op: effect.OpAdd, Value: tc.add}})
			}
			if got := c.InventoryLimit(); got != tc.want {
				t.Fatalf("InventoryLimit() = %d, want %d", got, tc.want)
			}
		})
	}
}

// TestAttachedInventoryCapacityFollowsInventoryLimit pins the player
// inventory's capacity check to the owner's live limit: items.size +
// slotCount <= InventoryLimit(), rechecked on every call.
func TestAttachedInventoryCapacityFollowsInventoryLimit(t *testing.T) {
	inv := itemcontainer.NewPlayerInventory(1, item.NewTable([]*item.Template{{ID: 1, Kind: item.KindWeapon, Weapon: &item.WeaponDetail{}}}))
	c := &Character{Race: RaceHuman}
	c.AttachRuntime(&Template{}, inv)
	c.Configure(Runtime{Rules: Rules{InventorySlots: InventorySlots{NoDwarf: 2, Dwarf: 5, Configured: true}}})
	inv.AddNew(1, 1, 101)

	if !inv.ValidateCapacity(1) {
		t.Fatal("ValidateCapacity(1) with 1 of 2 slots used = false, want true")
	}
	if inv.ValidateCapacity(2) {
		t.Fatal("ValidateCapacity(2) with 1 of 2 slots used = true, want false")
	}
	inv.AddNew(1, 1, 102)
	if inv.ValidateCapacity(1) {
		t.Fatal("ValidateCapacity(1) on a full inventory = true, want false")
	}
	if !inv.ValidateCapacity(0) {
		t.Fatal("ValidateCapacity(0) on a full inventory = false, want true")
	}

	c.AddStatFuncs([]effect.Mod{{Stat: stat.InvLim, Op: effect.OpAdd, Value: 1}})
	if !inv.ValidateCapacity(1) {
		t.Fatal("ValidateCapacity(1) after inventoryLimit +1 = false, want true")
	}
}
