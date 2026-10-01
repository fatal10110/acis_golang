package trade

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/privatestore"
)

const swordID = 30

// TestPrivateSellStoreSellsUntilEmpty walks a sell store end to end, as the
// reference runs it (SetPrivateStoreListSell, Player.onInteract,
// RequestPrivateStoreBuy, TradeList.privateStoreBuy): the owner lists 4 of
// its 10 potions and its sword; it sits down, its new state and title
// reach both clients; the buyer's second click opens the store window; a
// first purchase moves 3 potions for 150 adena with a message on each side;
// the second takes the last potion and the sword, which empties the list
// and closes the store for everyone. Every row lands in the database.
func TestPrivateSellStoreSellsUntilEmpty(t *testing.T) {
	t.Parallel()
	h := bootTraders(t)
	potions := h.srv.GiveItem(t, h.firstID, potionID, 10)
	sword := h.srv.GiveItem(t, h.firstID, swordID, 1)
	h.srv.GiveItem(t, h.secondID, item.AdenaID, 1000)
	h.enterAll(t)

	ownerFrames, buyerFrames := openSellStore(t, h, h.first, h.second, actionStoreSell, false,
		sellRow{potions, 4, 50}, sellRow{sword, 1, 300})
	assertOrder(t, ownerFrames, "owner opening", serverpackets.OpcodeChangeWaitType, serverpackets.OpcodeUserInfo, serverpackets.OpcodePrivateStoreMsgSell)
	assertOrder(t, buyerFrames, "buyer seeing the store open", serverpackets.OpcodeChangeWaitType, serverpackets.OpcodeCharInfo, serverpackets.OpcodePrivateStoreMsgSell)
	charInfoFor(t, buyerFrames, h.firstID)
	if got := h.srv.PlayerOperateType(t, h.firstID); got != privatestore.OperateSell {
		t.Fatalf("owner operate type = %d, want sell", got)
	}

	window := soleFrame(t, clickStore(t, h.second, h.firstID), serverpackets.OpcodePrivateStoreListSell, "store window")
	drainUntilQuiet(t, h.first) // the click's TargetSelected
	r := wire.NewReader(window[1:])
	if owner, packaged, adena, rows := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); owner != h.firstID || packaged != 0 || adena != 1000 || rows != 2 {
		t.Fatalf("store window owner %d packaged %d adena %d rows %d", owner, packaged, adena, rows)
	}

	h.second.Send(encodeRequestPrivateStoreBuy(h.firstID, sellRow{potions, 3, 50}))
	assertMessage(t, h.second.Read(), serverpackets.SystemMessagePurchasedS3S2SFromS1, textP("TraderOne"), itemNameP(potionID), numberP(3))
	assertMessage(t, h.first.Read(), serverpackets.SystemMessageS1PurchasedS3S2S, textP("TraderTwo"), itemNameP(potionID), numberP(3))
	h.srv.InventoryUpdates.Tick()
	h.srv.FlushPersistence(t)
	assertItemRows(t, h, h.firstID, "20x7 30x1 57x150")
	assertItemRows(t, h, h.secondID, "57x850 20x3")
	drainUntilQuiet(t, h.first)
	drainUntilQuiet(t, h.second)

	h.second.Send(encodeRequestPrivateStoreBuy(h.firstID, sellRow{potions, 1, 50}, sellRow{sword, 1, 300}))
	assertMessage(t, h.second.Read(), serverpackets.SystemMessagePurchasedS3S2SFromS1, textP("TraderOne"), itemNameP(potionID), numberP(1))
	assertMessage(t, h.second.Read(), serverpackets.SystemMessagePurchasedS2FromS1, textP("TraderOne"), itemNameP(swordID))
	assertMessage(t, h.first.Read(), serverpackets.SystemMessageS1PurchasedS3S2S, textP("TraderTwo"), itemNameP(potionID), numberP(1))
	assertMessage(t, h.first.Read(), serverpackets.SystemMessageS1PurchasedS2, textP("TraderTwo"), itemNameP(swordID))
	h.srv.Settle(t)
	if got := h.srv.PlayerOperateType(t, h.firstID); got != privatestore.OperateNone {
		t.Fatalf("owner operate type after selling out = %d, want none", got)
	}
	assertOrder(t, drainFrames(t, h.first), "owner store closing", serverpackets.OpcodeUserInfo)
	charInfoFor(t, drainFrames(t, h.second), h.firstID)
	h.srv.InventoryUpdates.Tick()
	h.srv.FlushPersistence(t)
	assertItemRows(t, h, h.firstID, "20x6 57x500")
	assertItemRows(t, h, h.secondID, "57x500 20x4 30x1")
}

// TestPrivateSellStoreRefusals pins the purchases a sell store refuses.
// The reference refuses a row at another price, and a buyer short of adena
// with YOU_NOT_ENOUGH_ADENA. Rows taking more units than the owner listed,
// or a package sale bought in part, are refused too: the reference lets
// both through, handing over units the owner never priced (see the PR's
// abuse notes); the client never sends either. No refusal moves an item.
func TestPrivateSellStoreRefusals(t *testing.T) {
	t.Parallel()
	h := bootTraders(t)
	potions := h.srv.GiveItem(t, h.firstID, potionID, 10)
	sword := h.srv.GiveItem(t, h.firstID, swordID, 1)
	h.srv.GiveItem(t, h.secondID, item.AdenaID, 100)
	h.enterAll(t)
	openSellStore(t, h, h.first, h.second, actionStoreSell, false, sellRow{potions, 4, 50}, sellRow{sword, 1, 300})

	for _, tc := range []struct {
		name string
		rows []sellRow
	}{
		{"another price", []sellRow{{potions, 1, 49}}},
		{"more units than listed", []sellRow{{potions, 5, 1}}},
		{"an unlisted item", []sellRow{{potions + 1000, 1, 50}}},
	} {
		h.second.Send(encodeRequestPrivateStoreBuy(h.firstID, tc.rows...))
		assertSilent(t, h.second, tc.name)
		assertSilent(t, h.first, tc.name+" (owner)")
	}
	h.second.Send(encodeRequestPrivateStoreBuy(h.firstID, sellRow{sword, 1, 300}))
	assertStaticSystemMessage(t, h.second.Read(), serverpackets.SystemMessageYouNotEnoughAdena)
	assertSilent(t, h.first, "owner of a sale the buyer cannot pay")

	h.srv.InventoryUpdates.Tick()
	h.srv.FlushPersistence(t)
	assertItemRows(t, h, h.firstID, "20x10 30x1")
	assertItemRows(t, h, h.secondID, "57x100")
}

// TestPackageSellStoreSellsOnlyWhole opens a package sale (action 61): the
// window reports the package flag, a purchase of part of the package is
// refused, and buying every row whole empties and closes the store.
func TestPackageSellStoreSellsOnlyWhole(t *testing.T) {
	t.Parallel()
	h := bootTraders(t)
	potions := h.srv.GiveItem(t, h.firstID, potionID, 10)
	sword := h.srv.GiveItem(t, h.firstID, swordID, 1)
	h.srv.GiveItem(t, h.secondID, item.AdenaID, 1000)
	h.enterAll(t)
	openSellStore(t, h, h.first, h.second, actionPackageSell, true, sellRow{potions, 4, 50}, sellRow{sword, 1, 300})
	if got := h.srv.PlayerOperateType(t, h.firstID); got != privatestore.OperatePackageSell {
		t.Fatalf("owner operate type = %d, want package sell", got)
	}
	window := soleFrame(t, clickStore(t, h.second, h.firstID), serverpackets.OpcodePrivateStoreListSell, "package window")
	drainUntilQuiet(t, h.first) // the click's TargetSelected
	if packaged := wire.NewReader(window[5:]).ReadInt32(); packaged != 1 {
		t.Fatalf("package window flag = %d, want 1", packaged)
	}

	for name, rows := range map[string][]sellRow{
		"one row of two":  {{sword, 1, 300}},
		"short of a row":  {{potions, 3, 50}, {sword, 1, 300}},
		"rows padded out": {{sword, 1, 300}, {sword, 1, 300}},
	} {
		h.second.Send(encodeRequestPrivateStoreBuy(h.firstID, rows...))
		assertSilent(t, h.second, name)
	}

	h.second.Send(encodeRequestPrivateStoreBuy(h.firstID, sellRow{potions, 4, 50}, sellRow{sword, 1, 300}))
	assertMessage(t, h.second.Read(), serverpackets.SystemMessagePurchasedS3S2SFromS1, textP("TraderOne"), itemNameP(potionID), numberP(4))
	assertMessage(t, h.second.Read(), serverpackets.SystemMessagePurchasedS2FromS1, textP("TraderOne"), itemNameP(swordID))
	h.srv.Settle(t)
	if got := h.srv.PlayerOperateType(t, h.firstID); got != privatestore.OperateNone {
		t.Fatalf("owner operate type after the package sold = %d, want none", got)
	}
	h.srv.InventoryUpdates.Tick()
	h.srv.FlushPersistence(t)
	assertItemRows(t, h, h.firstID, "20x6 57x500")
	assertItemRows(t, h, h.secondID, "57x500 20x4 30x1")
}

// TestSellStoreListRefusals pins SetPrivateStoreListSell's refusals: a
// list naming more units than the owner holds closes the store silently; a
// row that cannot be listed (an item listed twice past what is held)
// answers EXCEEDED_THE_MAXIMUM and shows the manage window again with what
// did get listed, the owner still setting up.
func TestSellStoreListRefusals(t *testing.T) {
	t.Parallel()
	h := bootTraders(t)
	potions := h.srv.GiveItem(t, h.firstID, potionID, 10)
	h.enterAll(t)

	h.first.Send(encodeRequestActionUse(actionStoreSell))
	assertFrameOpcode(t, h.first.Read(), serverpackets.OpcodePrivateStoreManageListSell, "PrivateStoreManageListSell")
	h.first.Send(encodeSetPrivateStoreListSell(false, sellRow{potions, 11, 5}))
	assertSilent(t, h.first, "list past the held count")
	if got := h.srv.PlayerOperateType(t, h.firstID); got != privatestore.OperateNone {
		t.Fatalf("operate type after an over-count list = %d, want none", got)
	}

	h.first.Send(encodeRequestActionUse(actionStoreSell))
	assertFrameOpcode(t, h.first.Read(), serverpackets.OpcodePrivateStoreManageListSell, "PrivateStoreManageListSell")
	h.first.Send(encodeSetPrivateStoreListSell(false, sellRow{potions, 6, 5}, sellRow{potions, 6, 5}))
	assertStaticSystemMessage(t, h.first.Read(), serverpackets.SystemMessageExceededTheMaximum)
	manage := h.first.Read()
	assertFrameOpcode(t, manage, serverpackets.OpcodePrivateStoreManageListSell, "manage window again")
	r := wire.NewReader(manage[1:])
	r.ReadInt32() // owner
	r.ReadInt32() // package flag
	r.ReadInt32() // adena
	if offered := r.ReadInt32(); offered != 1 {
		t.Fatalf("manage window offers %d items, want the potions' unlisted units", offered)
	}
	r.ReadInt32() // type2
	r.ReadInt32() // object
	r.ReadInt32() // item
	if left := r.ReadInt32(); left != 4 {
		t.Fatalf("manage window offers %d potions, want the 4 left unlisted", left)
	}
	if got := h.srv.PlayerOperateType(t, h.firstID); got != privatestore.OperateSellManage {
		t.Fatalf("operate type after an exceeded list = %d, want sell manage", got)
	}
}
