package trade

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/entity"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// zaricheID is the first shipped cursed weapon; no trader holds one here.
const zaricheID = 8190

// TestTradeSettlesWithCursedWeaponsConfigured pins that a direct trade moving
// items settles while the cursed-weapon table is loaded, as it is by
// default. The settle-time re-check asks, for every offered item, whether it
// is a cursed weapon while both inventories are held for the exchange; that
// question must be answered from the held contents, or the trade never
// finishes and both inventories stay locked. A held item configured as
// cursed that is not offered changes nothing.
func TestTradeSettlesWithCursedWeaponsConfigured(t *testing.T) {
	t.Parallel()
	table, err := entity.NewCursedWeaponTable([]entity.CursedWeapon{{ItemID: zaricheID}, {ItemID: heavyIngot}})
	if err != nil {
		t.Fatalf("NewCursedWeaponTable() error: %v", err)
	}
	h := bootTraders(t, gameservertest.WithCursedWeapons(table))
	adena := h.srv.GiveItem(t, h.firstID, item.AdenaID, 100)
	h.srv.GiveItem(t, h.firstID, heavyIngot, 1)
	potions := h.srv.GiveItem(t, h.secondID, potionID, 3)
	h.enterAll(t)
	h.startTrade(t)
	offerBoth(t, h, adena, 40, potions, 2)

	confirmBoth(t, h)
	assertTradeSucceeded(t, h)
	h.srv.Settle(t)

	for _, tc := range []struct {
		name               string
		ownerID            int32
		adena, potions, hi int
	}{
		{"first", h.firstID, 60, 2, 1},
		{"second", h.secondID, 40, 1, 0},
	} {
		inv := h.srv.PlayerInventory(t, tc.ownerID)
		if got := inv.ItemCount(item.AdenaID, -1, true); got != tc.adena {
			t.Fatalf("%s adena = %d, want %d", tc.name, got, tc.adena)
		}
		if got := inv.ItemCount(potionID, -1, true); got != tc.potions {
			t.Fatalf("%s potions = %d, want %d", tc.name, got, tc.potions)
		}
		if got := inv.ItemCount(heavyIngot, -1, true); got != tc.hi {
			t.Fatalf("%s ingots = %d, want %d", tc.name, got, tc.hi)
		}
	}

	h.srv.FlushPersistence(t)
	assertItemRows(t, h, h.firstID, "57x60 20x2 9500x1")
	assertItemRows(t, h, h.secondID, "57x40 20x1")
}
