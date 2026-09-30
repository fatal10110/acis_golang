package items

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestStorageLimitsFollowConfigAndLimitStats pins every ExStorageMaxCount
// field to the configured players.properties base plus its limit stat: on
// entry a non-dwarf reports the non-default configured bases, and putting
// on armor that carries every limit stat resends each base plus its bonus.
func TestStorageLimitsFollowConfigAndLimitStats(t *testing.T) {
	t.Parallel()
	var templates []*item.Template
	for _, tmpl := range gameservertest.ItemTemplates().All() {
		if tmpl.ID == inventoryLimitArmorID {
			clone := *tmpl
			clone.Modifiers = append(append([]item.StatModifier(nil), tmpl.Modifiers...),
				item.StatModifier{Op: item.FuncAdd, Stat: "inventoryLimit", Value: 1},
				item.StatModifier{Op: item.FuncAdd, Stat: "whLimit", Value: 2},
				item.StatModifier{Op: item.FuncAdd, Stat: "FreightLimit", Value: 3},
				item.StatModifier{Op: item.FuncAdd, Stat: "PrivateSellLimit", Value: 4},
				item.StatModifier{Op: item.FuncAdd, Stat: "PrivateBuyLimit", Value: 5},
				item.StatModifier{Op: item.FuncAdd, Stat: "DwarfRecipeLimit", Value: 6},
				item.StatModifier{Op: item.FuncAdd, Stat: "CommonRecipeLimit", Value: 7})
			tmpl = &clone
		}
		templates = append(templates, tmpl)
	}
	srv := gameservertest.Boot(t,
		gameservertest.WithItemTemplates(item.NewTable(templates)),
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithInventorySlots(90, 117),
		gameservertest.WithStorageSlots(player.StorageSlots{
			WarehouseNoDwarf: 31, WarehouseDwarf: 32, Freight: 33,
			PrivateStoreNoDwarf: 34, PrivateStoreDwarf: 35,
			DwarfRecipe: 36, CommonRecipe: 37,
		}),
	)
	c := srv.Client
	objID := srv.SoleObjectID(t)
	armor := srv.GiveItem(t, objID, inventoryLimitArmorID, 1)
	burst := startInWorld(t, c)

	assertStorageLimits(t, burst[1], [7]int32{90, 31, 33, 34, 34, 36, 37})

	c.Send(encodeUseItem(armor, false))
	frames := collectUntilQuiet(t, c)
	idx := storageLimitFrame(frames)
	if idx < 0 {
		t.Fatal("equipping the limit-stat armor sent no ExStorageMaxCount")
	}
	assertStorageLimits(t, frames[idx], [7]int32{91, 33, 36, 38, 39, 42, 44})
}

// assertStorageLimits requires f to be ExStorageMaxCount reporting want as
// inventory, warehouse, freight, private sell, private buy, dwarven recipe
// and common recipe limits.
func assertStorageLimits(t *testing.T, f []byte, want [7]int32) {
	t.Helper()
	if !isStorageLimitFrame(f) {
		t.Fatalf("frame %x is not ExStorageMaxCount", f)
	}
	r := wire.NewReader(f[3:])
	var got [7]int32
	for i := range got {
		got[i] = r.ReadInt32()
	}
	if err := r.Err(); err != nil {
		t.Fatalf("read ExStorageMaxCount: %v", err)
	}
	if got != want {
		t.Fatalf("ExStorageMaxCount limits = %v, want %v", got, want)
	}
}
