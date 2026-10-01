package trade

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/privatestore"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// openBuyStore takes the owner through the buy command and the list.
func openBuyStore(t *testing.T, h *traders, rows ...buyRow) (ownerFrames, otherFrames [][]byte) {
	t.Helper()
	h.first.Send(encodeRequestActionUse(actionStoreBuy))
	assertFrameOpcode(t, h.first.Read(), serverpackets.OpcodePrivateStoreManageListBuy, "PrivateStoreManageListBuy")
	h.first.Send(encodeSetPrivateStoreListBuy(rows...))
	h.srv.Settle(t)
	return drainFrames(t, h.first), drainFrames(t, h.second)
}

// TestPrivateBuyStoreBuysUntilFilled walks a buy store end to end
// (SetPrivateStoreListBuy, Player.onInteract, RequestPrivateStoreSell,
// TradeList.privateStoreSell): the owner, holding one potion, wants 5 more
// at 30 each; the seller's window shows its own potions against the row; a
// sale of 3 pays the seller 90 adena with a message on each side; a sale
// past what is still wanted is refused; the last 2 fill the list and close
// the store.
func TestPrivateBuyStoreBuysUntilFilled(t *testing.T) {
	t.Parallel()
	h := bootTraders(t)
	h.srv.GiveItem(t, h.firstID, potionID, 1)
	h.srv.GiveItem(t, h.firstID, item.AdenaID, 1000)
	sellerPotions := h.srv.GiveItem(t, h.secondID, potionID, 10)
	h.enterAll(t)

	h.first.Send(encodeStoreText(clientpackets.OpcodeSetPrivateStoreMsgBuy, "potions wanted"))
	title := h.first.Read()
	assertFrameOpcode(t, title, serverpackets.OpcodePrivateStoreMsgBuy, "own buy title")
	ownerFrames, sellerFrames := openBuyStore(t, h, buyRow{potionID, 0, 5, 30})
	assertOrder(t, ownerFrames, "owner opening", serverpackets.OpcodeChangeWaitType, serverpackets.OpcodeUserInfo, serverpackets.OpcodePrivateStoreMsgBuy)
	shown := soleFrame(t, sellerFrames, serverpackets.OpcodePrivateStoreMsgBuy, "seller sees the title")
	r := wire.NewReader(shown[1:])
	if owner, text := r.ReadInt32(), r.ReadString(); owner != h.firstID || text != "potions wanted" {
		t.Fatalf("PrivateStoreMsgBuy = %d %q", owner, text)
	}

	window := soleFrame(t, clickStore(t, h.second, h.firstID), serverpackets.OpcodePrivateStoreListBuy, "store window")
	drainUntilQuiet(t, h.first) // the click's TargetSelected
	r = wire.NewReader(window[1:])
	if owner, adena, rows := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); owner != h.firstID || adena != 0 || rows != 1 {
		t.Fatalf("store window owner %d adena %d rows %d", owner, adena, rows)
	}
	objectID, itemID := r.ReadInt32(), r.ReadInt32()
	r.ReadUint16() // enchant
	count := r.ReadInt32()
	if objectID != sellerPotions || itemID != potionID || count != 5 {
		t.Fatalf("store window row = object %d item %d count %d, want the seller's potions, 5", objectID, itemID, count)
	}

	h.second.Send(encodeRequestPrivateStoreSell(h.firstID, saleRow{sellerPotions, potionID, 0, 3, 30}))
	assertMessage(t, h.second.Read(), serverpackets.SystemMessageS1PurchasedS3S2S, textP("TraderOne"), itemNameP(potionID), numberP(3))
	assertMessage(t, h.first.Read(), serverpackets.SystemMessagePurchasedS3S2SFromS1, textP("TraderTwo"), itemNameP(potionID), numberP(3))

	// Only 2 are still wanted, and the owner priced nothing else.
	for name, row := range map[string]saleRow{
		"past what is wanted": {sellerPotions, potionID, 0, 3, 30},
		"another price":       {sellerPotions, potionID, 0, 1, 31},
		"another enchant":     {sellerPotions, potionID, 1, 1, 30},
	} {
		h.second.Send(encodeRequestPrivateStoreSell(h.firstID, row))
		assertSilent(t, h.second, name)
	}

	h.second.Send(encodeRequestPrivateStoreSell(h.firstID, saleRow{sellerPotions, potionID, 0, 2, 30}))
	assertMessage(t, h.second.Read(), serverpackets.SystemMessageS1PurchasedS3S2S, textP("TraderOne"), itemNameP(potionID), numberP(2))
	h.srv.Settle(t)
	if got := h.srv.PlayerOperateType(t, h.firstID); got != privatestore.OperateNone {
		t.Fatalf("owner operate type after the list filled = %d, want none", got)
	}
	h.srv.InventoryUpdates.Tick()
	h.srv.FlushPersistence(t)
	assertItemRows(t, h, h.firstID, "20x6 57x850")
	assertItemRows(t, h, h.secondID, "20x5 57x150")
}

// TestBuyStoreListRefusals pins SetPrivateStoreListBuy's refusals: wanting
// an item the owner holds none of closes the store silently; a list
// costing more adena than the owner holds answers
// THE_PURCHASE_PRICE_IS_HIGHER_THAN_MONEY and shows the manage window again;
// selling into it then finds no store.
func TestBuyStoreListRefusals(t *testing.T) {
	t.Parallel()
	h := bootTraders(t)
	h.srv.GiveItem(t, h.firstID, potionID, 1)
	h.srv.GiveItem(t, h.firstID, item.AdenaID, 100)
	h.enterAll(t)

	h.first.Send(encodeRequestActionUse(actionStoreBuy))
	assertFrameOpcode(t, h.first.Read(), serverpackets.OpcodePrivateStoreManageListBuy, "PrivateStoreManageListBuy")
	h.first.Send(encodeSetPrivateStoreListBuy(buyRow{swordID, 0, 1, 10}))
	assertSilent(t, h.first, "wanting an item not held")
	if got := h.srv.PlayerOperateType(t, h.firstID); got != privatestore.OperateNone {
		t.Fatalf("operate type = %d, want none", got)
	}

	h.first.Send(encodeRequestActionUse(actionStoreBuy))
	assertFrameOpcode(t, h.first.Read(), serverpackets.OpcodePrivateStoreManageListBuy, "PrivateStoreManageListBuy")
	h.first.Send(encodeSetPrivateStoreListBuy(buyRow{potionID, 0, 5, 30}))
	assertStaticSystemMessage(t, h.first.Read(), serverpackets.SystemMessagePurchasePriceHigherThanMoney)
	assertFrameOpcode(t, h.first.Read(), serverpackets.OpcodePrivateStoreManageListBuy, "manage window again")
	if got := h.srv.PlayerOperateType(t, h.firstID); got != privatestore.OperateBuyManage {
		t.Fatalf("operate type = %d, want buy manage", got)
	}
}

// TestPrivateStoreTransactionRight pins the access-level gate on every
// store call site: a character whose access level forbids transactions
// cannot list a sell or buy store (the store closes with
// YOU_ARE_NOT_AUTHORIZED_TO_DO_THAT) and cannot buy from or sell into
// another player's store.
func TestPrivateStoreTransactionRight(t *testing.T) {
	t.Parallel()
	t.Run("listing", func(t *testing.T) {
		h := bootTraders(t, gameservertest.WithAdmin(shippedAccessLevels(t)))
		setCharacterColumn(t, h, h.firstID, "accesslevel", testGMLevel)
		potions := h.srv.GiveItem(t, h.firstID, potionID, 10)
		h.enterAll(t)

		h.first.Send(encodeRequestActionUse(actionStoreSell))
		assertFrameOpcode(t, h.first.Read(), serverpackets.OpcodePrivateStoreManageListSell, "PrivateStoreManageListSell")
		h.first.Send(encodeSetPrivateStoreListSell(false, sellRow{potions, 1, 5}))
		assertStaticSystemMessage(t, h.first.Read(), serverpackets.SystemMessageNotAuthorizedToDoThat)

		h.first.Send(encodeRequestActionUse(actionStoreBuy))
		assertFrameOpcode(t, h.first.Read(), serverpackets.OpcodePrivateStoreManageListBuy, "PrivateStoreManageListBuy")
		h.first.Send(encodeSetPrivateStoreListBuy(buyRow{potionID, 0, 1, 0}))
		assertStaticSystemMessage(t, h.first.Read(), serverpackets.SystemMessageNotAuthorizedToDoThat)
		if got := h.srv.PlayerOperateType(t, h.firstID); got != privatestore.OperateNone {
			t.Fatalf("operate type = %d, want none", got)
		}
	})
	t.Run("dealing", func(t *testing.T) {
		h := bootTraders(t, gameservertest.WithAdmin(shippedAccessLevels(t)))
		setCharacterColumn(t, h, h.secondID, "accesslevel", testGMLevel)
		potions := h.srv.GiveItem(t, h.firstID, potionID, 10)
		h.srv.GiveItem(t, h.secondID, item.AdenaID, 100)
		h.enterAll(t)
		openSellStore(t, h, h.first, h.second, actionStoreSell, false, sellRow{potions, 4, 5})

		h.second.Send(encodeRequestPrivateStoreBuy(h.firstID, sellRow{potions, 1, 5}))
		assertStaticSystemMessage(t, h.second.Read(), serverpackets.SystemMessageNotAuthorizedToDoThat)
		h.first.Send(encodeRequestPrivateStoreQuitSell())
		h.srv.Settle(t)
		drainUntilQuiet(t, h.first)
		drainUntilQuiet(t, h.second)

		h.srv.SetPlayerOperateType(t, h.firstID, privatestore.OperateBuy)
		h.second.Send(encodeRequestPrivateStoreSell(h.firstID, saleRow{potions, potionID, 0, 1, 5}))
		assertStaticSystemMessage(t, h.second.Read(), serverpackets.SystemMessageNotAuthorizedToDoThat)
		h.srv.InventoryUpdates.Tick()
		h.srv.FlushPersistence(t)
		assertItemRows(t, h, h.firstID, "20x10")
	})
}

// TestTradeRequestRefusedAroundStores pins the store-mode trade gates: a
// trade request to or from a player setting up a store answers
// PRIVATE_STORE_UNDER_WAY, to or from one running a store
// CANNOT_TRADE_DISCARD_DROP_ITEM_WHILE_IN_SHOPMODE; the target hears
// nothing either way.
func TestTradeRequestRefusedAroundStores(t *testing.T) {
	t.Parallel()
	h := bootTraders(t)
	h.enterAll(t)
	for _, tc := range []struct {
		name    string
		who     int32
		operate privatestore.OperateType
		message int
	}{
		{"target setting up", h.secondID, privatestore.OperateSellManage, serverpackets.SystemMessagePrivateStoreUnderWay},
		{"requester setting up", h.firstID, privatestore.OperateManufactureManage, serverpackets.SystemMessagePrivateStoreUnderWay},
		{"target selling", h.secondID, privatestore.OperateSell, serverpackets.SystemMessageCannotTradeDiscardDropInShopMode},
		{"requester buying", h.firstID, privatestore.OperateBuy, serverpackets.SystemMessageCannotTradeDiscardDropInShopMode},
	} {
		h.srv.SetPlayerOperateType(t, tc.who, tc.operate)
		h.first.Send(encodeTradeRequest(h.secondID))
		assertStaticSystemMessage(t, h.first.Read(), tc.message)
		assertSilent(t, h.second, tc.name)
		h.srv.SetPlayerOperateType(t, tc.who, privatestore.OperateNone)
	}
}

func encodeRequestPrivateStoreQuitSell() []byte {
	return wire.NewPacketWriter(clientpackets.OpcodeRequestPrivateStoreQuitSell).Bytes()
}
