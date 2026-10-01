package network

import (
	"errors"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
)

// dispatchPrivateStore decodes one private store or workshop request and
// runs it on live's queue. It reports false when a malformed packet ended
// the connection. A request with no player in the world is dropped.
func (l *GameClientLink) dispatchPrivateStore(client *Client, live *livePlayer, opcode byte, payload []byte) bool {
	maxItems := l.playerConfig.InventorySlots.MaxItemInPacket()
	var run func()
	var err error
	switch opcode {
	case clientpackets.OpcodeRequestPrivateStoreManageSell:
		run = func() { l.tryOpenSellStore(live, false) }
	case clientpackets.OpcodeRequestPrivateStoreManageBuy:
		run = func() { l.tryOpenBuyStore(live) }
	case clientpackets.OpcodeRequestPrivateStoreQuitSell, clientpackets.OpcodeRequestPrivateStoreQuitBuy,
		clientpackets.OpcodeRequestRecipeShopManageQuit:
		run = func() { l.quitPrivateStore(live) }
	case clientpackets.OpcodeRequestRecipeShopManagePrev:
		run = func() { l.showWorkshopOf(live) }
	case clientpackets.OpcodeSetPrivateStoreMsgSell, clientpackets.OpcodeSetPrivateStoreMsgBuy,
		clientpackets.OpcodeRequestRecipeShopMessageSet:
		var req clientpackets.StoreMessage
		req, err = decodeClientPacket(l, client, payload, clientpackets.DecodeStoreMessage)
		run = func() {
			switch opcode {
			case clientpackets.OpcodeSetPrivateStoreMsgSell:
				l.setSellStoreTitle(live, req)
			case clientpackets.OpcodeSetPrivateStoreMsgBuy:
				l.setBuyStoreTitle(live, req)
			default:
				l.setWorkshopName(live, req)
			}
		}
	case clientpackets.OpcodeSetPrivateStoreListSell:
		var req clientpackets.SetPrivateStoreListSell
		req, err = decodeClientPacket(l, client, payload, func(p []byte) (clientpackets.SetPrivateStoreListSell, error) {
			return clientpackets.DecodeSetPrivateStoreListSell(p, maxItems)
		})
		run = func() { l.setSellStoreList(live, req) }
	case clientpackets.OpcodeSetPrivateStoreListBuy:
		var req clientpackets.SetPrivateStoreListBuy
		req, err = decodeClientPacket(l, client, payload, func(p []byte) (clientpackets.SetPrivateStoreListBuy, error) {
			return clientpackets.DecodeSetPrivateStoreListBuy(p, maxItems)
		})
		run = func() { l.setBuyStoreList(live, req) }
	case clientpackets.OpcodeRequestPrivateStoreBuy:
		var req clientpackets.RequestPrivateStoreBuy
		req, err = decodeClientPacket(l, client, payload, func(p []byte) (clientpackets.RequestPrivateStoreBuy, error) {
			return clientpackets.DecodeRequestPrivateStoreBuy(p, maxItems)
		})
		run = func() { l.buyFromStore(live, req) }
	case clientpackets.OpcodeRequestPrivateStoreSell:
		var req clientpackets.RequestPrivateStoreSell
		req, err = decodeClientPacket(l, client, payload, func(p []byte) (clientpackets.RequestPrivateStoreSell, error) {
			return clientpackets.DecodeRequestPrivateStoreSell(p, maxItems)
		})
		run = func() { l.sellToStore(live, req) }
	case clientpackets.OpcodeRequestRecipeShopListSet:
		var req clientpackets.RequestRecipeShopListSet
		req, err = decodeClientPacket(l, client, payload, func(p []byte) (clientpackets.RequestRecipeShopListSet, error) {
			return clientpackets.DecodeRequestRecipeShopListSet(p, maxItems)
		})
		run = func() { l.setWorkshopList(live, req) }
	case clientpackets.OpcodeRequestRecipeShopMakeInfo:
		var req clientpackets.RequestRecipeShopMakeInfo
		req, err = decodeClientPacket(l, client, payload, clientpackets.DecodeRequestRecipeShopMakeInfo)
		run = func() { l.showWorkshopRecipe(live, req) }
	case clientpackets.OpcodeRequestRecipeShopMakeItem:
		var req clientpackets.RequestRecipeShopMakeItem
		req, err = decodeClientPacket(l, client, payload, clientpackets.DecodeRequestRecipeShopMakeItem)
		// An order inside the manufacture reuse window, which self crafts
		// share, is dropped silently before anything else is looked at.
		if err == nil && !client.performFloodProtected(floodProtectorManufacture, l.playerConfig.ManufactureDelay, time.Now()) {
			return true
		}
		run = func() { l.orderWorkshopCraft(live, req) }
	default:
		return true
	}
	if err != nil {
		return !errors.Is(err, errMalformedPacketDisconnect)
	}
	if live != nil {
		onLive(live, run)
	}
	return true
}
