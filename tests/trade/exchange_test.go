package trade

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestTradeExchangeTransfersItemsAndPersists walks the full direct-trade
// flow end to end: request/answer handshake opens a window on both clients,
// each side offers stackable items (offer packets mirrored to the partner),
// both confirm, and the exchange commits with SendTradeDone success,
// TradeSuccessful messages, InventoryUpdate frames on both sides, and
// persisted items rows whose owners and counts reflect the swap.
func TestTradeExchangeTransfersItemsAndPersists(t *testing.T) {
	h := bootTraders(t)
	adena := h.srv.GiveItem(t, h.firstID, item.AdenaID, 100)
	potions := h.srv.GiveItem(t, h.secondID, 20, 3)
	h.enterAll(t)
	h.startTrade(t)

	// The first trader offers 40 of his 100 adena.
	h.first.Send(encodeAddTradeItem(0, adena, 40))
	assertOwnOfferFrames(t, h.first.Read(), h.first.Read(), h.first.Read(), adena, item.AdenaID, 40, 60)
	frame := h.second.Read()
	assertFrameOpcode(t, frame, serverpackets.OpcodeTradeOtherAdd, "second TradeOtherAdd")
	assertTradeAddRow(t, frame, serverpackets.OpcodeTradeOtherAdd, item.AdenaID, 40)
	drainUntilQuiet(t, h.second)

	// The second trader offers 2 of his 3 potions back.
	h.second.Send(encodeAddTradeItem(0, potions, 2))
	assertOwnOfferFrames(t, h.second.Read(), h.second.Read(), h.second.Read(), potions, 20, 2, 1)
	frame = h.first.Read()
	assertFrameOpcode(t, frame, serverpackets.OpcodeTradeOtherAdd, "first TradeOtherAdd")
	assertTradeAddRow(t, frame, serverpackets.OpcodeTradeOtherAdd, 20, 2)
	drainUntilQuiet(t, h.first)

	// First confirm: the confirmer sees its own press, the partner the
	// confirmed message plus the other-side press.
	h.first.Send(encodeTradeDone(1))
	assertFrameOpcode(t, h.first.Read(), serverpackets.OpcodeTradePressOwnOk, "first TradePressOwnOk")
	assertSystemMessageText(t, h.second.Read(), serverpackets.SystemMessageS1ConfirmedTrade, "TraderOne")
	assertFrameOpcode(t, h.second.Read(), serverpackets.OpcodeTradePressOtherOk, "second TradePressOtherOk")

	// Second confirm settles the exchange.
	h.second.Send(encodeTradeDone(1))
	for _, who := range []struct {
		name   string
		client *testsupport.ScriptedClient
	}{
		{"first", h.first},
		{"second", h.second},
	} {
		frame := who.client.Read()
		assertFrameOpcode(t, frame, serverpackets.OpcodeSendTradeDone, who.name+" SendTradeDone")
		if got := wire.NewReader(frame[1:]).ReadInt32(); got != 1 {
			t.Fatalf("%s SendTradeDone success = %d, want 1", who.name, got)
		}
		assertStaticSystemMessage(t, who.client.Read(), serverpackets.SystemMessageTradeSuccessful)
	}
	// Inventory mutations reach the clients through the lazy-persistence
	// tick's update fan-out.
	h.srv.InventoryUpdates.Tick()
	firstFrames := drainFrames(t, h.first)
	secondFrames := drainFrames(t, h.second)

	findInventoryUpdate(t, firstFrames, adena, 60)
	findAddedInventoryUpdate(t, firstFrames, 20, 2)
	findInventoryUpdate(t, secondFrames, potions, 1)
	transferredAdena := findAddedInventoryUpdate(t, secondFrames, item.AdenaID, 40)
	if transferredAdena == adena {
		t.Fatalf("transferred adena reused source object id %d", adena)
	}

	h.srv.FlushItems(t)
	ctx := context.Background()
	firstRows, err := h.srv.Items.ListByOwner(ctx, h.firstID)
	if err != nil {
		t.Fatalf("list first items: %v", err)
	}
	if len(firstRows) != 2 {
		t.Fatalf("persisted first rows = %+v, want two", firstRows)
	}
	for _, row := range firstRows {
		switch {
		case row.TemplateID == item.AdenaID:
			if row.ObjectID != adena || row.Count != 60 || row.OwnerID != h.firstID {
				t.Fatalf("persisted adena remainder = %+v, want object %d count 60", row, adena)
			}
		case row.TemplateID == 20:
			if row.ObjectID == potions || row.Count != 2 || row.OwnerID != h.firstID {
				t.Fatalf("persisted received potions = %+v, want fresh object %d count 2", row, potions)
			}
		default:
			t.Fatalf("unexpected persisted first row %+v", row)
		}
	}
	secondRows, err := h.srv.Items.ListByOwner(ctx, h.secondID)
	if err != nil {
		t.Fatalf("list second items: %v", err)
	}
	if len(secondRows) != 2 {
		t.Fatalf("persisted second rows = %+v, want two", secondRows)
	}
	for _, row := range secondRows {
		switch {
		case row.TemplateID == item.AdenaID:
			if row.ObjectID != transferredAdena || row.Count != 40 || row.OwnerID != h.secondID {
				t.Fatalf("persisted transferred adena = %+v, want object %d count 40 owner %d",
					row, transferredAdena, h.secondID)
			}
		case row.TemplateID == 20:
			if row.ObjectID != potions || row.Count != 1 || row.OwnerID != h.secondID {
				t.Fatalf("persisted potion remainder = %+v, want object %d count 1", row, potions)
			}
		default:
			t.Fatalf("unexpected persisted row %+v", row)
		}
	}
}

// assertOwnOfferFrames asserts the three-packet answer an add-item request
// produces on the offering client: TradeOwnAdd for the offered quantity,
// TradeUpdate carrying the remaining available count, and TradeItemUpdate.
func assertOwnOfferFrames(t *testing.T, ownAdd, update, itemUpdate []byte, objectID, templateID, offered, remaining int32) {
	t.Helper()
	assertFrameOpcode(t, ownAdd, serverpackets.OpcodeTradeOwnAdd, "TradeOwnAdd")
	assertTradeAddRow(t, ownAdd, serverpackets.OpcodeTradeOwnAdd, templateID, offered)

	assertFrameOpcode(t, update, serverpackets.OpcodeTradeUpdate, "TradeUpdate")
	r := wire.NewReader(update[1:])
	r.ReadUint16() // row count
	r.ReadUint16() // mode
	r.ReadUint16() // item category
	r.ReadInt32()  // object id
	r.ReadInt32()  // template id
	if got := r.ReadInt32(); got != remaining {
		t.Fatalf("TradeUpdate available count = %d, want %d", got, remaining)
	}
	if err := r.Err(); err != nil {
		t.Fatalf("read TradeUpdate: %v", err)
	}

	assertFrameOpcode(t, itemUpdate, serverpackets.OpcodeTradeItemUpdate, "TradeItemUpdate")
}

// assertTradeAddRow checks a TradeOwnAdd/TradeOtherAdd frame carries the
// given template and quantity.
func assertTradeAddRow(t *testing.T, frame []byte, opcode byte, templateID, quantity int32) {
	t.Helper()
	r := wire.NewReader(frame[1:])
	r.ReadUint16() // row count
	r.ReadUint16() // item category
	r.ReadInt32()  // object id
	if got := r.ReadInt32(); got != templateID {
		t.Fatalf("trade add opcode %#x template id = %d, want %d", opcode, got, templateID)
	}
	if got := r.ReadInt32(); got != quantity {
		t.Fatalf("trade add opcode %#x quantity = %d, want %d", opcode, got, quantity)
	}
	if err := r.Err(); err != nil {
		t.Fatalf("read trade add row: %v", err)
	}
}

// inventoryEntry is one update row inside InventoryUpdate.
type inventoryEntry struct {
	state uint16 // 1 added, 2 modified, 3 removed
	objID int32
	item  int32
	count int32
}

func readInventoryUpdateEntries(t *testing.T, frame []byte) []inventoryEntry {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeInventoryUpdate, "InventoryUpdate")
	r := wire.NewReader(frame[1:])
	n := r.ReadUint16()
	entries := make([]inventoryEntry, 0, n)
	for i := uint16(0); i < n; i++ {
		var e inventoryEntry
		e.state = r.ReadUint16()
		r.ReadUint16() // item category
		e.objID = r.ReadInt32()
		e.item = r.ReadInt32()
		e.count = r.ReadInt32()
		r.ReadUint16() // subCategory
		r.ReadUint16() // CustomType1
		r.ReadUint16() // equipped
		r.ReadInt32()  // paperdoll slot
		r.ReadUint16() // enchant
		r.ReadUint16() // CustomType2
		r.ReadInt32()  // augmentation
		r.ReadInt32()  // mana left
		entries = append(entries, e)
	}
	if err := r.Err(); err != nil {
		t.Fatalf("read InventoryUpdate: %v", err)
	}
	return entries
}

// findInventoryUpdate scans frames for an InventoryUpdate entry reporting
// objectID with wantCount.
func findInventoryUpdate(t *testing.T, frames [][]byte, objectID, wantCount int32) inventoryEntry {
	t.Helper()
	for _, frame := range frames {
		if len(frame) == 0 || frame[0] != serverpackets.OpcodeInventoryUpdate {
			continue
		}
		for _, e := range readInventoryUpdateEntries(t, frame) {
			if e.objID == objectID && e.count == wantCount {
				return e
			}
		}
	}
	t.Fatalf("no InventoryUpdate entry for object %d count %d in %d frames", objectID, wantCount, len(frames))
	return inventoryEntry{}
}

// findAddedInventoryUpdate scans frames for an added InventoryUpdate entry of
// the given template and count, returning its new object id.
func findAddedInventoryUpdate(t *testing.T, frames [][]byte, templateID, wantCount int32) int32 {
	t.Helper()
	for _, frame := range frames {
		if len(frame) == 0 || frame[0] != serverpackets.OpcodeInventoryUpdate {
			continue
		}
		for _, e := range readInventoryUpdateEntries(t, frame) {
			if e.item == templateID && e.state == 1 && e.count == wantCount {
				return e.objID
			}
		}
	}
	t.Fatalf("no added InventoryUpdate entry for template %d count %d in %d frames", templateID, wantCount, len(frames))
	return 0
}
