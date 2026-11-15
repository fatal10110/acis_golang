package pets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

const (
	giveRodID  = int32(6529)
	giveLureID = int32(6519)
)

// rodAndLureCatalog is the shared catalog with a fishing rod and a stackable
// lure a pet may be handed.
func rodAndLureCatalog() *item.Table {
	return item.NewTable(append(gameservertest.ItemTemplates().All(),
		&item.Template{
			ID: giveRodID, Name: "Baby Duck Rod", Kind: item.KindWeapon, Slot: item.SlotLRHand, Duration: -1,
			Dropable: true, Tradable: true, Destroyable: true,
			Weapon: &item.WeaponDetail{Type: item.WeaponFishingRod},
		},
		&item.Template{
			ID: giveLureID, Name: "Fishing Lure", Kind: item.KindEtcItem, Slot: item.SlotLHand, Duration: -1,
			Stackable: true, Dropable: true, Tradable: true, Destroyable: true, DefaultAction: item.ActionEquip,
			EtcItem: &item.EtcItemDetail{Type: item.EtcItemLure},
		},
	))
}

// TestGivePartOfWornStackToPetLeavesRestWorn has the owner fish with ten
// lures worn, then hand one to the wolf. Only the whole stack would come
// off: the nine left stay worn in the left hand.
func TestGivePartOfWornStackToPetLeavesRestWorn(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollarOpts(t, []gameservertest.Option{gameservertest.WithItemTemplates(rodAndLureCatalog())},
		seedItem{TemplateID: giveRodID, Count: 1}, seedItem{TemplateID: giveLureID, Count: 10})
	h.spawnWolf(t)
	h.settleInventoryUpdates(t)
	rod, lure := h.seededItem(t, giveRodID), h.seededItem(t, giveLureID)
	h.sendAndTick(t, encodeUseItem(rod, false))
	h.sendAndTick(t, encodeUseItem(lure, false))
	inv := h.srv.PlayerInventory(t, h.ownerID)
	if worn := inv.ItemAt(itemcontainer.LHand); worn == nil || worn.ObjectID != lure {
		t.Fatalf("left hand before the give = %v, want the lure %d", worn, lure)
	}

	h.sendAndTick(t, encodeRequestGiveItemToPet(lure, 1))
	if got := h.collarItemCount(t, giveLureID); got != 1 {
		t.Fatalf("pet lures = %d, want 1", got)
	}
	if got := h.ownerItemCount(t, giveLureID); got != 9 {
		t.Fatalf("owner lures = %d, want 9", got)
	}
	if worn := inv.ItemAt(itemcontainer.LHand); worn == nil || worn.ObjectID != lure {
		t.Fatalf("left hand after giving one lure = %v, want the nine left still worn", worn)
	}
}
