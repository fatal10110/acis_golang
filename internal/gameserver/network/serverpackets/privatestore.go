package serverpackets

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/recipe"
	"github.com/fatal10110/acis_golang/internal/gameserver/privatestore"
)

// Private store and workshop opcodes.
const (
	OpcodePrivateStoreManageListSell = 0x9a
	OpcodePrivateStoreListSell       = 0x9b
	OpcodePrivateStoreMsgSell        = 0x9c
	OpcodePrivateStoreManageListBuy  = 0xb7
	OpcodePrivateStoreListBuy        = 0xb8
	OpcodePrivateStoreMsgBuy         = 0xb9
	OpcodeRecipeShopManageList       = 0xd8
	OpcodeRecipeShopSellList         = 0xd9
	OpcodeRecipeShopItemInfo         = 0xda
	OpcodeRecipeShopMsg              = 0xdb
)

// FramePrivateStoreManageListSell builds the sell store's manage window:
// the owner's adena, the items it may still list (each at its reference
// price) and the rows already listed.
func FramePrivateStoreManageListSell(ownerID int32, packaged bool, adena int, candidates []privatestore.SellCandidate, listed []privatestore.SellItem, templates *item.Table) (wire.Frame, error) {
	w := newFrameWriter(OpcodePrivateStoreManageListSell)
	w.WriteInt32(ownerID)
	w.WriteInt32(boolInt32(packaged))
	w.WriteInt32(int32(adena))
	w.WriteInt32(int32(len(candidates)))
	for _, c := range candidates {
		tmpl, ok := templates.Get(c.Item.TemplateID)
		if !ok {
			releaseFrameWriter(w)
			return wire.Frame{}, fmt.Errorf("serverpackets: PrivateStoreManageListSell: no template loaded for item template %d", c.Item.TemplateID)
		}
		writeStoreSellRow(w, tmpl, c.Item.ObjectID, c.Count, c.Item.EnchantLevel, tmpl.ReferencePrice)
	}
	w.WriteInt32(int32(len(listed)))
	for _, row := range listed {
		tmpl, ok := templates.Get(row.TemplateID)
		if !ok {
			releaseFrameWriter(w)
			return wire.Frame{}, fmt.Errorf("serverpackets: PrivateStoreManageListSell: no template loaded for item template %d", row.TemplateID)
		}
		writeStoreSellRow(w, tmpl, row.ObjectID, row.Count, row.Enchant, int32(row.Price))
		w.WriteInt32(tmpl.ReferencePrice)
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter), nil
}

// FramePrivateStoreListSell builds a sell store's window as buyer sees it:
// the store's owner, whether it sells as one package, the buyer's adena and
// every listed row with its price and reference price.
func FramePrivateStoreListSell(ownerID int32, packaged bool, buyerAdena int, listed []privatestore.SellItem, templates *item.Table) (wire.Frame, error) {
	w := newFrameWriter(OpcodePrivateStoreListSell)
	w.WriteInt32(ownerID)
	w.WriteInt32(boolInt32(packaged))
	w.WriteInt32(int32(buyerAdena))
	w.WriteInt32(int32(len(listed)))
	for _, row := range listed {
		tmpl, ok := templates.Get(row.TemplateID)
		if !ok {
			releaseFrameWriter(w)
			return wire.Frame{}, fmt.Errorf("serverpackets: PrivateStoreListSell: no template loaded for item template %d", row.TemplateID)
		}
		writeStoreSellRow(w, tmpl, row.ObjectID, row.Count, row.Enchant, int32(row.Price))
		w.WriteInt32(tmpl.ReferencePrice)
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter), nil
}

// writeStoreSellRow writes the item fields every sell-store row shares:
// its type, object, item, count, enchant, body part and price.
func writeStoreSellRow(w *wire.Writer, tmpl *item.Template, objectID int32, count, enchant int, price int32) {
	_, type2 := tmpl.Category()
	w.WriteInt32(int32(type2))
	w.WriteInt32(objectID)
	w.WriteInt32(tmpl.ID)
	w.WriteInt32(int32(count))
	w.WriteUint16(0)
	w.WriteUint16(uint16(enchant))
	w.WriteUint16(0)
	w.WriteInt32(int32(tmpl.Slot))
	w.WriteInt32(price)
}

// FramePrivateStoreManageListBuy builds the buy store's manage window: the
// owner's adena, the items it may want (one per item that does not stack)
// and the rows already wanted.
func FramePrivateStoreManageListBuy(ownerID int32, adena int, candidates []item.InstanceState, listed []privatestore.BuyItem, templates *item.Table) (wire.Frame, error) {
	w := newFrameWriter(OpcodePrivateStoreManageListBuy)
	w.WriteInt32(ownerID)
	w.WriteInt32(int32(adena))
	w.WriteInt32(int32(len(candidates)))
	for _, st := range candidates {
		tmpl, ok := templates.Get(st.TemplateID)
		if !ok {
			releaseFrameWriter(w)
			return wire.Frame{}, fmt.Errorf("serverpackets: PrivateStoreManageListBuy: no template loaded for item template %d", st.TemplateID)
		}
		writeStoreBuyRow(w, tmpl, st.EnchantLevel, st.Count)
	}
	w.WriteInt32(int32(len(listed)))
	for _, row := range listed {
		tmpl, ok := templates.Get(row.TemplateID)
		if !ok {
			releaseFrameWriter(w)
			return wire.Frame{}, fmt.Errorf("serverpackets: PrivateStoreManageListBuy: no template loaded for item template %d", row.TemplateID)
		}
		writeStoreBuyRow(w, tmpl, row.Enchant, row.Quantity)
		w.WriteInt32(int32(row.Price))
		w.WriteInt32(tmpl.ReferencePrice)
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter), nil
}

// FramePrivateStoreListBuy builds a buy store's window as seller sees it:
// the store's owner, the seller's adena and every wanted row, showing the
// seller's own instance that can fill it, how many units of it the row
// takes, the price and the units still wanted.
func FramePrivateStoreListBuy(ownerID int32, sellerAdena int, offers []privatestore.BuyOffer, templates *item.Table) (wire.Frame, error) {
	w := newFrameWriter(OpcodePrivateStoreListBuy)
	w.WriteInt32(ownerID)
	w.WriteInt32(int32(sellerAdena))
	w.WriteInt32(int32(len(offers)))
	for _, offer := range offers {
		tmpl, ok := templates.Get(offer.TemplateID)
		if !ok {
			releaseFrameWriter(w)
			return wire.Frame{}, fmt.Errorf("serverpackets: PrivateStoreListBuy: no template loaded for item template %d", offer.TemplateID)
		}
		w.WriteInt32(offer.ObjectID)
		writeStoreBuyRow(w, tmpl, offer.Enchant, offer.Count)
		w.WriteInt32(int32(offer.Price))
		w.WriteInt32(int32(offer.Quantity))
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter), nil
}

// writeStoreBuyRow writes the item fields every buy-store row shares: its
// item, enchant, count, reference price, body part and type.
func writeStoreBuyRow(w *wire.Writer, tmpl *item.Template, enchant, count int) {
	_, type2 := tmpl.Category()
	w.WriteInt32(tmpl.ID)
	w.WriteUint16(uint16(enchant))
	w.WriteInt32(int32(count))
	w.WriteInt32(tmpl.ReferencePrice)
	w.WriteUint16(0)
	w.WriteInt32(int32(tmpl.Slot))
	w.WriteUint16(uint16(type2))
}

// FramePrivateStoreMsgSell builds the sell store title shown over its
// owner.
func FramePrivateStoreMsgSell(ownerID int32, title string) wire.Frame {
	return frameStoreMessage(OpcodePrivateStoreMsgSell, ownerID, title)
}

// FramePrivateStoreMsgBuy builds the buy store title shown over its owner.
func FramePrivateStoreMsgBuy(ownerID int32, title string) wire.Frame {
	return frameStoreMessage(OpcodePrivateStoreMsgBuy, ownerID, title)
}

// FrameRecipeShopMsg builds the workshop name shown over its owner.
func FrameRecipeShopMsg(ownerID int32, name string) wire.Frame {
	return frameStoreMessage(OpcodeRecipeShopMsg, ownerID, name)
}

func frameStoreMessage(opcode byte, ownerID int32, text string) wire.Frame {
	w := newFrameWriter(opcode)
	w.WriteInt32(ownerID)
	w.WriteString(text)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameRecipeShopManageList builds the workshop's manage window: the
// owner's adena, the page it is set up from (0 dwarven, 1 common), every
// recipe of the shown book page with its 1-based position, and the
// workshop's recipes on that page with their costs.
func FrameRecipeShopManageList(ownerID int32, adena int, dwarven bool, book []recipe.Recipe, offered []privatestore.ManufactureItem) wire.Frame {
	w := newFrameWriter(OpcodeRecipeShopManageList)
	w.WriteInt32(ownerID)
	w.WriteInt32(int32(adena))
	if dwarven {
		w.WriteInt32(0)
	} else {
		w.WriteInt32(1)
	}
	w.WriteInt32(int32(len(book)))
	for i, r := range book {
		w.WriteInt32(int32(r.ID))
		w.WriteInt32(int32(i + 1))
	}
	writeManufactureItems(w, offered)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameRecipeShopSellList builds a workshop's window as a customer sees it:
// the crafter, its current and max MP, the customer's adena and every
// recipe the workshop offers with its cost.
func FrameRecipeShopSellList(crafterID, mp, maxMP int32, customerAdena int, offered []privatestore.ManufactureItem) wire.Frame {
	w := newFrameWriter(OpcodeRecipeShopSellList)
	w.WriteInt32(crafterID)
	w.WriteInt32(mp)
	w.WriteInt32(maxMP)
	w.WriteInt32(int32(customerAdena))
	writeManufactureItems(w, offered)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

func writeManufactureItems(w *wire.Writer, offered []privatestore.ManufactureItem) {
	w.WriteInt32(int32(len(offered)))
	for _, m := range offered {
		w.WriteInt32(int32(m.RecipeID))
		w.WriteInt32(0)
		w.WriteInt32(int32(m.Cost))
	}
}

// FrameRecipeShopItemInfo builds a workshop's craft window for recipeID:
// the crafter and its current and max MP.
func FrameRecipeShopItemInfo(crafterID, recipeID, mp, maxMP int32) wire.Frame {
	w := newFrameWriter(OpcodeRecipeShopItemInfo)
	w.WriteInt32(crafterID)
	w.WriteInt32(recipeID)
	w.WriteInt32(mp)
	w.WriteInt32(maxMP)
	w.WriteInt32(-1)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}
