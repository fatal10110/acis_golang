package serverpackets

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/buylist"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

const (
	// OpcodeSellList is the wire opcode for the merchant sell-to-NPC item list.
	OpcodeSellList = 0x10
	// OpcodeBuyList is the wire opcode for the merchant buy-from-NPC item list.
	OpcodeBuyList = 0x11
	// OpcodeTradeStart is the wire opcode for a newly opened direct-trade window.
	OpcodeTradeStart = 0x1e
	// OpcodeTradeOwnAdd is the wire opcode for adding an item on the sender side.
	OpcodeTradeOwnAdd = 0x20
	// OpcodeTradeOtherAdd is the wire opcode for adding an item on the partner side.
	OpcodeTradeOtherAdd = 0x21
	// OpcodeSendTradeDone is the wire opcode for final trade result.
	OpcodeSendTradeDone = 0x22
	// OpcodeSendTradeRequest is the wire opcode for a pending direct-trade request.
	OpcodeSendTradeRequest = 0x5e
	// OpcodeTradePressOwnOk is the wire opcode shown when the local player confirms.
	OpcodeTradePressOwnOk = 0x75
	// OpcodeTradePressOtherOk is the wire opcode shown when the trade partner confirms.
	OpcodeTradePressOtherOk = 0x7c
	// OpcodeTradeItemUpdate is the wire opcode for refreshing trade-offer availability.
	OpcodeTradeItemUpdate = 0x74
	// OpcodeTradeUpdate is the wire opcode for one trade-offer availability row.
	OpcodeTradeUpdate = 0x74
)

// TradeItemSnapshot is one item row shown in direct-trade packets.
type TradeItemSnapshot struct {
	ObjectID     int32
	TemplateID   int32
	Count        int
	EnchantLevel int
}

// TradeItemUpdateEntry is one row in the trade item availability refresh.
type TradeItemUpdateEntry struct {
	Item           TradeItemSnapshot
	AvailableCount int
}

// FrameBuyList builds the BuyList packet for a merchant buylist.
func FrameBuyList(list buylist.List, currentMoney int, taxRate float64, templates *item.Table) (wire.Frame, error) {
	w := newFrameWriter(OpcodeBuyList)
	if err := writeBuyList(w, list, currentMoney, taxRate, templates); err != nil {
		releaseFrameWriter(w)
		return wire.Frame{}, err
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter), nil
}

func writeBuyList(w *wire.Writer, list buylist.List, currentMoney int, taxRate float64, templates *item.Table) error {
	w.WriteInt32(int32(currentMoney))
	w.WriteInt32(int32(list.ID))
	w.WriteUint16(uint16(len(list.Products)))
	for _, product := range list.Products {
		if product.LimitedStock() && product.MaxCount <= 0 {
			continue
		}
		tmpl, ok := templates.Get(product.ItemID)
		if !ok {
			return fmt.Errorf("serverpackets: BuyList: no template loaded for item template %d", product.ItemID)
		}
		count := product.MaxCount
		if count < 0 {
			count = 0
		}
		price := int32(float64(product.Price) * (1 + taxRate))
		writeShopItem(w, tmpl, product.ItemID, product.ItemID, count, 0, 0, 0, price)
	}
	return nil
}

// FrameSellList builds the SellList packet for items the player can offer
// to a merchant.
func FrameSellList(currentMoney int, items []*item.Instance, templates *item.Table) (wire.Frame, error) {
	w := newFrameWriter(OpcodeSellList)
	if err := writeSellList(w, currentMoney, items, templates); err != nil {
		releaseFrameWriter(w)
		return wire.Frame{}, err
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter), nil
}

func writeSellList(w *wire.Writer, currentMoney int, items []*item.Instance, templates *item.Table) error {
	w.WriteInt32(int32(currentMoney))
	w.WriteInt32(0)
	w.WriteUint16(uint16(len(items)))
	for _, inst := range items {
		st := inst.Snapshot()
		tmpl, ok := templates.Get(st.TemplateID)
		if !ok {
			return fmt.Errorf("serverpackets: SellList: no template loaded for item template %d", st.TemplateID)
		}
		writeShopItem(w, tmpl, st.ObjectID, st.TemplateID, st.Count, st.EnchantLevel, st.CustomType1, st.CustomType2, tmpl.ReferencePrice/2)
	}
	return nil
}

// FrameSendTradeRequest builds the packet asking the receiver to accept a
// direct-trade request from senderID.
func FrameSendTradeRequest(senderID int32) wire.Frame {
	w := newFrameWriter(OpcodeSendTradeRequest)
	w.WriteInt32(senderID)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameSendTradeDone builds the final direct-trade result packet.
func FrameSendTradeDone(success bool) wire.Frame {
	w := newFrameWriter(OpcodeSendTradeDone)
	w.WriteInt32(boolInt32(success))
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameTradePressOwnOk builds the packet marking the local player's trade
// confirmation.
func FrameTradePressOwnOk() wire.Frame {
	w := newFrameWriter(OpcodeTradePressOwnOk)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameTradePressOtherOk builds the packet marking the trade partner's
// confirmation.
func FrameTradePressOtherOk() wire.Frame {
	w := newFrameWriter(OpcodeTradePressOtherOk)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter)
}

// FrameTradeStart builds the initial direct-trade item list.
func FrameTradeStart(partnerID int32, items []*item.Instance, templates *item.Table) (wire.Frame, error) {
	w := newFrameWriter(OpcodeTradeStart)
	if err := writeTradeStart(w, partnerID, items, templates); err != nil {
		releaseFrameWriter(w)
		return wire.Frame{}, err
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter), nil
}

func writeTradeStart(w *wire.Writer, partnerID int32, items []*item.Instance, templates *item.Table) error {
	available, err := availableTradeStartItems(items, templates)
	if err != nil {
		return err
	}
	w.WriteInt32(partnerID)
	w.WriteUint16(uint16(len(available)))
	for _, row := range available {
		inst := row.inst
		tmpl := row.tmpl
		st := inst.Snapshot()
		writeTradeItem(w, tmpl, st.ObjectID, st.TemplateID, st.Count, st.EnchantLevel)
	}
	return nil
}

// FrameTradeOwnAdd builds the direct-trade row for an item the local player
// added.
func FrameTradeOwnAdd(it TradeItemSnapshot, quantity int, templates *item.Table) (wire.Frame, error) {
	return frameTradeAdd(OpcodeTradeOwnAdd, it, quantity, templates)
}

// FrameTradeOtherAdd builds the direct-trade row for an item the trade
// partner added.
func FrameTradeOtherAdd(it TradeItemSnapshot, quantity int, templates *item.Table) (wire.Frame, error) {
	return frameTradeAdd(OpcodeTradeOtherAdd, it, quantity, templates)
}

// FrameTradeUpdate builds a one-row direct-trade availability update.
func FrameTradeUpdate(it TradeItemSnapshot, quantity int, templates *item.Table) (wire.Frame, error) {
	w := newFrameWriter(OpcodeTradeUpdate)
	tmpl, ok := templates.Get(it.TemplateID)
	if !ok {
		releaseFrameWriter(w)
		return wire.Frame{}, fmt.Errorf("serverpackets: TradeUpdate: no template loaded for item template %d", it.TemplateID)
	}
	w.WriteUint16(1)
	w.WriteUint16(tradeUpdateMode(tmpl, quantity))
	writeTradeItem(w, tmpl, it.ObjectID, it.TemplateID, quantity, it.EnchantLevel)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter), nil
}

// FrameTradeItemUpdate builds the full direct-trade availability refresh.
func FrameTradeItemUpdate(entries []TradeItemUpdateEntry, templates *item.Table) (wire.Frame, error) {
	w := newFrameWriter(OpcodeTradeItemUpdate)
	if err := writeTradeItemUpdate(w, entries, templates); err != nil {
		releaseFrameWriter(w)
		return wire.Frame{}, err
	}
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter), nil
}

func writeTradeItemUpdate(w *wire.Writer, entries []TradeItemUpdateEntry, templates *item.Table) error {
	w.WriteUint16(uint16(len(entries)))
	for _, entry := range entries {
		tmpl, ok := templates.Get(entry.Item.TemplateID)
		if !ok {
			return fmt.Errorf("serverpackets: TradeItemUpdate: no template loaded for item template %d", entry.Item.TemplateID)
		}
		available := entry.AvailableCount
		stackable := tmpl.Stackable
		if available == 0 {
			available = 1
			stackable = false
		}
		mode := uint16(2)
		if stackable {
			mode = 3
		}
		w.WriteUint16(mode)
		writeTradeItem(w, tmpl, entry.Item.ObjectID, entry.Item.TemplateID, available, entry.Item.EnchantLevel)
	}
	return nil
}

func frameTradeAdd(opcode byte, it TradeItemSnapshot, quantity int, templates *item.Table) (wire.Frame, error) {
	w := newFrameWriter(opcode)
	tmpl, ok := templates.Get(it.TemplateID)
	if !ok {
		releaseFrameWriter(w)
		return wire.Frame{}, fmt.Errorf("serverpackets: TradeAdd: no template loaded for item template %d", it.TemplateID)
	}
	w.WriteUint16(1)
	writeTradeItem(w, tmpl, it.ObjectID, it.TemplateID, quantity, it.EnchantLevel)
	return wire.OwnedFrame(w.Frame(), w, releaseFrameWriter), nil
}

func tradeUpdateMode(tmpl *item.Template, quantity int) uint16 {
	if quantity > 0 && tmpl.Stackable {
		return 3
	}
	return 2
}

func writeShopItem(w *wire.Writer, tmpl *item.Template, objectID, templateID int32, count, enchant, custom1, custom2 int, price int32) {
	category, subCategory := tmpl.Category()
	w.WriteUint16(uint16(category))
	w.WriteInt32(objectID)
	w.WriteInt32(templateID)
	w.WriteInt32(int32(count))
	w.WriteUint16(uint16(subCategory))
	w.WriteUint16(uint16(custom1))
	w.WriteInt32(int32(tmpl.Slot))
	w.WriteUint16(uint16(enchant))
	w.WriteUint16(uint16(custom2))
	w.WriteUint16(0)
	w.WriteInt32(price)
}

func writeTradeItem(w *wire.Writer, tmpl *item.Template, objectID, templateID int32, count, enchant int) {
	category, subCategory := tmpl.Category()
	w.WriteUint16(uint16(category))
	w.WriteInt32(objectID)
	w.WriteInt32(templateID)
	w.WriteInt32(int32(count))
	w.WriteUint16(uint16(subCategory))
	w.WriteUint16(0)
	w.WriteInt32(int32(tmpl.Slot))
	w.WriteUint16(uint16(enchant))
	w.WriteUint16(0)
	w.WriteUint16(0)
}

type tradeStartItem struct {
	inst *item.Instance
	tmpl *item.Template
}

func availableTradeStartItems(items []*item.Instance, templates *item.Table) ([]tradeStartItem, error) {
	out := make([]tradeStartItem, 0, len(items))
	for _, inst := range items {
		if inst == nil {
			continue
		}
		st := inst.Snapshot()
		if st.Location != item.LocationInventory {
			continue
		}
		tmpl, ok := templates.Get(st.TemplateID)
		if !ok {
			return nil, fmt.Errorf("serverpackets: TradeStart: no template loaded for item template %d", st.TemplateID)
		}
		if !inst.Tradable(tmpl) || inst.QuestItem(tmpl) {
			continue
		}
		out = append(out, tradeStartItem{inst: inst, tmpl: tmpl})
	}
	return out, nil
}
