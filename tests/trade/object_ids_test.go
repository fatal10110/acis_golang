package trade

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// Reference: every trade, private store and workshop row moves through
// ItemContainer.transferItem (TradeList.java:350, 484, 608), which creates
// an instance, and so takes an object id, only when the move leaves part of
// its source behind in a target holding no stack of it
// (ItemContainer.java:255-305).

// TestTradeTakesObjectIDsOnlyForSplitRows settles trades whose rows split,
// land on a held stack, or move whole, and counts the object ids each took.
// The last cases give the first trader's whole adena stack away and take
// some back on the same trade: the stack is gone by the time the returning
// row moves, so that row starts a new one, even when it offered its whole
// stack, which the first stack has just grown.
func TestTradeTakesObjectIDsOnlyForSplitRows(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name                      string
		firstHolds, secondHolds   [2]int32 // template, count
		firstOffers, secondOffers int32
		wantIDs                   int
		firstRows, secondRows     string
	}{
		{
			name:       "adena into a partner holding none, a whole sword back",
			firstHolds: [2]int32{item.AdenaID, 100}, firstOffers: 40,
			secondHolds: [2]int32{swordID, 1}, secondOffers: 1,
			wantIDs:   1, // the adena splits into a partner holding none
			firstRows: "57x60 30x1", secondRows: "57x40",
		},
		{
			name:       "two splits into partners holding none",
			firstHolds: [2]int32{item.AdenaID, 100}, firstOffers: 40,
			secondHolds: [2]int32{potionID, 3}, secondOffers: 2,
			wantIDs:   2,
			firstRows: "57x60 20x2", secondRows: "57x40 20x1",
		},
		{
			name:       "adena both ways onto held stacks",
			firstHolds: [2]int32{item.AdenaID, 100}, firstOffers: 40,
			secondHolds: [2]int32{item.AdenaID, 50}, secondOffers: 10,
			wantIDs:   0,
			firstRows: "57x70", secondRows: "57x80",
		},
		{
			name:       "a whole stack away, part of one back",
			firstHolds: [2]int32{item.AdenaID, 100}, firstOffers: 100,
			secondHolds: [2]int32{item.AdenaID, 50}, secondOffers: 10,
			wantIDs:   1,
			firstRows: "57x10", secondRows: "57x140",
		},
		{
			// The first stack grows the second, so the second's whole
			// offer is now part of it.
			name:       "both whole adena stacks",
			firstHolds: [2]int32{item.AdenaID, 100}, firstOffers: 100,
			secondHolds: [2]int32{item.AdenaID, 50}, secondOffers: 50,
			wantIDs:   1,
			firstRows: "57x50", secondRows: "57x100",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := bootTraders(t)
			firstItem := h.srv.GiveItem(t, h.firstID, tc.firstHolds[0], tc.firstHolds[1])
			secondItem := h.srv.GiveItem(t, h.secondID, tc.secondHolds[0], tc.secondHolds[1])
			h.enterAll(t)
			h.startTrade(t)
			offerBoth(t, h, firstItem, tc.firstOffers, secondItem, tc.secondOffers)

			before := h.srv.IssuedObjectIDs()
			confirmBoth(t, h)
			assertTradeSucceeded(t, h)
			h.srv.Settle(t)
			if got := h.srv.IssuedObjectIDs() - before; got != tc.wantIDs {
				t.Fatalf("the trade took %d object ids, want %d", got, tc.wantIDs)
			}
			landItemRows(t, h)
			assertItemRows(t, h, h.firstID, tc.firstRows)
			assertItemRows(t, h, h.secondID, tc.secondRows)
		})
	}
}

// TestTradeOfAWholeSwordOntoAdenaTakesNoObjectID trades a whole sword for
// adena landing on the seller's held stack: neither row creates an
// instance, so the trade takes no object id at all.
func TestTradeOfAWholeSwordOntoAdenaTakesNoObjectID(t *testing.T) {
	t.Parallel()
	h := bootTraders(t)
	adena := h.srv.GiveItem(t, h.firstID, item.AdenaID, 100)
	h.srv.GiveItem(t, h.secondID, item.AdenaID, 5)
	sword := h.srv.GiveItem(t, h.secondID, swordID, 1)
	h.enterAll(t)
	h.startTrade(t)
	offerBoth(t, h, adena, 40, sword, 1)

	before := h.srv.IssuedObjectIDs()
	confirmBoth(t, h)
	assertTradeSucceeded(t, h)
	h.srv.Settle(t)
	if got := h.srv.IssuedObjectIDs() - before; got != 0 {
		t.Fatalf("the trade took %d object ids, want 0", got)
	}
	landItemRows(t, h)
	assertItemRows(t, h, h.firstID, "57x60 30x1")
	assertItemRows(t, h, h.secondID, "57x45")
}

// TestPrivateStoresTakeObjectIDsOnlyForSplitRows buys from a sell store
// and sells to a buy store, counting the object ids each deal took: the
// payment lands on the owner's or seller's held adena and takes none, an
// item row starting the receiver's stack takes one, and a row landing on
// it or moving whole takes none.
func TestPrivateStoresTakeObjectIDsOnlyForSplitRows(t *testing.T) {
	t.Parallel()
	t.Run("sell store", func(t *testing.T) {
		t.Parallel()
		h := bootTraders(t)
		potions := h.srv.GiveItem(t, h.firstID, potionID, 10)
		sword := h.srv.GiveItem(t, h.firstID, swordID, 1)
		h.srv.GiveItem(t, h.firstID, item.AdenaID, 10)
		h.srv.GiveItem(t, h.secondID, item.AdenaID, 1000)
		h.enterAll(t)
		openSellStore(t, h, h.first, h.second, actionStoreSell, false, sellRow{potions, 4, 50}, sellRow{sword, 1, 300})
		soleFrame(t, clickStore(t, h.second, h.firstID), serverpackets.OpcodePrivateStoreListSell, "store window")
		drainUntilQuiet(t, h.first)

		buy := func(wantIDs int, rows ...sellRow) {
			t.Helper()
			before := h.srv.IssuedObjectIDs()
			h.second.Send(encodeRequestPrivateStoreBuy(h.firstID, rows...))
			h.srv.Settle(t)
			drainUntilQuiet(t, h.first)
			drainUntilQuiet(t, h.second)
			if got := h.srv.IssuedObjectIDs() - before; got != wantIDs {
				t.Fatalf("buying %v took %d object ids, want %d", rows, got, wantIDs)
			}
		}
		buy(1, sellRow{potions, 3, 50})                         // the potions start the buyer's stack
		buy(0, sellRow{potions, 1, 50}, sellRow{sword, 1, 300}) // onto that stack, and a whole sword
		h.srv.InventoryUpdates.Tick()
		landItemRows(t, h)
		assertItemRows(t, h, h.firstID, "20x6 57x510")
		assertItemRows(t, h, h.secondID, "57x500 20x4 30x1")
	})
	t.Run("buy store", func(t *testing.T) {
		t.Parallel()
		h := bootTraders(t)
		h.srv.GiveItem(t, h.firstID, potionID, 1)
		h.srv.GiveItem(t, h.firstID, item.AdenaID, 1000)
		sellerPotions := h.srv.GiveItem(t, h.secondID, potionID, 10)
		h.srv.GiveItem(t, h.secondID, item.AdenaID, 5)
		h.enterAll(t)
		openBuyStore(t, h, buyRow{potionID, 0, 5, 30})
		soleFrame(t, clickStore(t, h.second, h.firstID), serverpackets.OpcodePrivateStoreListBuy, "store window")
		drainUntilQuiet(t, h.first)

		before := h.srv.IssuedObjectIDs()
		h.second.Send(encodeRequestPrivateStoreSell(h.firstID, saleRow{sellerPotions, potionID, 0, 3, 30}))
		h.srv.Settle(t)
		drainUntilQuiet(t, h.first)
		drainUntilQuiet(t, h.second)
		if got := h.srv.IssuedObjectIDs() - before; got != 0 {
			t.Fatalf("the sale took %d object ids, want 0", got)
		}
		h.srv.InventoryUpdates.Tick()
		landItemRows(t, h)
		assertItemRows(t, h, h.firstID, "20x4 57x910")
		assertItemRows(t, h, h.secondID, "20x7 57x95")
	})
}

// landItemRows waits until the rows a deal changed are in the database, so a
// suite can compare them with what the deal left in memory. The deal's own
// write is bounded by a short deadline; on a loaded database it can miss it,
// and the rows then land with the next lazy-persistence tick, as they would
// in production. Flushing the tick as well makes the rows read here the
// deal's outcome either way. Whether the deal's own write lands its rows is
// pinned by the atomic trade suites.
func landItemRows(t *testing.T, h *traders) {
	t.Helper()
	h.srv.FlushItems(t)
}
