//go:build integration

package network

import (
	"context"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	invops "github.com/fatal10110/acis_golang/internal/gameserver/inventory"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	tradebook "github.com/fatal10110/acis_golang/internal/gameserver/trade"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

func TestDirectTradeRequestAndAnswerStartsTrade(t *testing.T) {
	link, _, firstCap, secondCap, first, second := newDirectTradeFixture(t)

	link.handleTradeRequest(first, clientpackets.TradeRequest{ObjectID: second.ObjectID()})
	testsupport.AssertOpcodeSequence(t, secondCap.Frames(), serverpackets.OpcodeSendTradeRequest)
	assertSystemMessageStringFrame(t, firstCap.Frames()[0], serverpackets.SystemMessageRequestS1ForTrade, second.Name)

	link.handleAnswerTradeRequest(second, clientpackets.AnswerTradeRequest{Response: 1})

	testsupport.AssertOpcodeSequence(t, firstCap.Frames(),
		serverpackets.OpcodeSystemMessage,
		serverpackets.OpcodeSystemMessage,
		serverpackets.OpcodeTradeStart,
	)
	testsupport.AssertOpcodeSequence(t, secondCap.Frames(),
		serverpackets.OpcodeSendTradeRequest,
		serverpackets.OpcodeSystemMessage,
		serverpackets.OpcodeTradeStart,
	)
	assertSystemMessageStringFrame(t, firstCap.Frames()[1], serverpackets.SystemMessageBeginTradeWithS1, second.Name)
	assertSystemMessageStringFrame(t, secondCap.Frames()[1], serverpackets.SystemMessageBeginTradeWithS1, first.Name)

	if !link.trades.HasActive(first.ObjectID()) || !link.trades.HasActive(second.ObjectID()) {
		t.Fatal("trade session was not registered for both players")
	}
}

func TestDirectTradeAddItemSendsOfferPackets(t *testing.T) {
	link, _, firstCap, secondCap, first, second := newStartedDirectTradeFixture(t)
	seedLiveItem(t, first, 500, item.AdenaID, 100)
	testsupport.ResetCapture(firstCap, secondCap)

	link.handleAddTradeItem(first, clientpackets.AddTradeItem{ObjectID: 500, Count: 40})

	testsupport.AssertOpcodeSequence(t, firstCap.Frames(),
		serverpackets.OpcodeTradeOwnAdd,
		serverpackets.OpcodeTradeUpdate,
		serverpackets.OpcodeTradeItemUpdate,
	)
	testsupport.AssertOpcodeSequence(t, secondCap.Frames(), serverpackets.OpcodeTradeOtherAdd)
	assertTradeUpdateCount(t, firstCap.Frames()[1], 60)
	_ = second
}

func TestDirectTradeAddItemToleratesPartnerOutOfRange(t *testing.T) {
	link, _, firstCap, secondCap, first, second := newStartedDirectTradeFixture(t)
	seedLiveItem(t, first, 500, item.AdenaID, 100)
	if err := link.world.Move(second, 1000, 0, 0); err != nil {
		t.Fatalf("Move: %v", err)
	}
	resetCapture(firstCap, secondCap)

	link.handleAddTradeItem(first, clientpackets.AddTradeItem{ObjectID: 500, Count: 40})

	assertOpcodeSequence(t, firstCap.frames,
		serverpackets.OpcodeTradeOwnAdd,
		serverpackets.OpcodeTradeUpdate,
		serverpackets.OpcodeTradeItemUpdate,
	)
	assertOpcodeSequence(t, secondCap.frames, serverpackets.OpcodeTradeOtherAdd)
	if !link.trades.HasActive(first.ObjectID()) || !link.trades.HasActive(second.ObjectID()) {
		t.Fatal("trade session should still be active after AddTradeItem with partner out of range")
	}
}

func TestDirectTradeRejectsInvalidItem(t *testing.T) {
	link, _, firstCap, secondCap, first, _ := newStartedDirectTradeFixture(t)
	seedLiveItem(t, first, 501, 1463, 5)
	testsupport.ResetCapture(firstCap, secondCap)

	link.handleAddTradeItem(first, clientpackets.AddTradeItem{ObjectID: 501, Count: 1})

	testsupport.AssertOpcodeSequence(t, firstCap.Frames(), serverpackets.OpcodeSystemMessage)
	assertStaticSystemMessageFrame(t, firstCap.Frames()[0], serverpackets.SystemMessageNothingHappened)
	if len(secondCap.Frames()) != 0 {
		t.Fatalf("partner frames = %x, want none", testsupport.FrameOpcodes(secondCap.Frames()))
	}
}

func TestDirectTradeConfirmTransfersItemsAndPersists(t *testing.T) {
	link, store, firstCap, secondCap, first, second := newStartedDirectTradeFixture(t)
	ctx := context.Background()
	adena := seedLiveItem(t, first, 500, item.AdenaID, 100)
	if err := store.Create(ctx, first.ObjectID(), *adena); err != nil {
		t.Fatalf("seed store: %v", err)
	}
	testsupport.ResetCapture(firstCap, secondCap)

	link.handleAddTradeItem(first, clientpackets.AddTradeItem{ObjectID: 500, Count: 40})
	testsupport.ResetCapture(firstCap, secondCap)

	link.handleTradeDone(ctx, first, clientpackets.TradeDone{Response: 1})
	testsupport.AssertOpcodeSequence(t, firstCap.Frames(), serverpackets.OpcodeTradePressOwnOk)
	testsupport.AssertOpcodeSequence(t, secondCap.Frames(), serverpackets.OpcodeSystemMessage, serverpackets.OpcodeTradePressOtherOk)
	assertSystemMessageStringFrame(t, secondCap.Frames()[0], serverpackets.SystemMessageS1ConfirmedTrade, first.Name)
	testsupport.ResetCapture(firstCap, secondCap)

	link.handleTradeDone(ctx, second, clientpackets.TradeDone{Response: 1})
	link.inventoryUpdates.Tick()

	testsupport.AssertOpcodeSequence(t, firstCap.Frames(),
		serverpackets.OpcodeSendTradeDone,
		serverpackets.OpcodeSystemMessage,
		serverpackets.OpcodeInventoryUpdate,
	)
	testsupport.AssertOpcodeSequence(t, secondCap.Frames(),
		serverpackets.OpcodeSendTradeDone,
		serverpackets.OpcodeSystemMessage,
		serverpackets.OpcodeInventoryUpdate,
	)
	assertTradeDoneFrame(t, firstCap.Frames()[0], true)
	assertTradeDoneFrame(t, secondCap.Frames()[0], true)
	assertStaticSystemMessageFrame(t, firstCap.Frames()[1], serverpackets.SystemMessageTradeSuccessful)
	assertStaticSystemMessageFrame(t, secondCap.Frames()[1], serverpackets.SystemMessageTradeSuccessful)

	if got := first.Inventory().ItemByObjectID(500).Count; got != 60 {
		t.Fatalf("first adena count = %d, want 60", got)
	}
	if got := second.Inventory().ItemCount(item.AdenaID, -1, true); got != 40 {
		t.Fatalf("second adena count = %d, want 40", got)
	}
	firstRows, err := store.ListByOwner(ctx, first.ObjectID())
	if err != nil {
		t.Fatalf("list first items: %v", err)
	}
	if len(firstRows) != 1 || firstRows[0].ObjectID != 500 || firstRows[0].Count != 60 {
		t.Fatalf("persisted first items = %+v, want object 500 count 60", firstRows)
	}
	secondRows, err := store.ListByOwner(ctx, second.ObjectID())
	if err != nil {
		t.Fatalf("list second items: %v", err)
	}
	if len(secondRows) != 1 || secondRows[0].TemplateID != item.AdenaID || secondRows[0].Count != 40 || secondRows[0].OwnerID != second.ObjectID() {
		t.Fatalf("persisted second items = %+v, want transferred adena", secondRows)
	}
}

func TestDirectTradeCancelClearsSession(t *testing.T) {
	link, _, firstCap, secondCap, first, second := newStartedDirectTradeFixture(t)
	testsupport.ResetCapture(firstCap, secondCap)

	link.handleTradeDone(context.Background(), first, clientpackets.TradeDone{Response: 0})

	testsupport.AssertOpcodeSequence(t, firstCap.Frames(), serverpackets.OpcodeSendTradeDone, serverpackets.OpcodeSystemMessage)
	testsupport.AssertOpcodeSequence(t, secondCap.Frames(), serverpackets.OpcodeSendTradeDone, serverpackets.OpcodeSystemMessage)
	assertSystemMessageStringFrame(t, firstCap.Frames()[1], serverpackets.SystemMessageS1CanceledTrade, second.Name)
	assertSystemMessageStringFrame(t, secondCap.Frames()[1], serverpackets.SystemMessageS1CanceledTrade, first.Name)
	if link.trades.HasActive(first.ObjectID()) || link.trades.HasActive(second.ObjectID()) {
		t.Fatal("trade session was not cleared after cancel")
	}
}

func TestDirectTradeClientLoopDispatchesInGame(t *testing.T) {
	c, _, _, _, _, _ := newLinkedSQLGameClient(t, nil, nil, 0)

	c.Send(encodeRequestCharacterCreate("Newbie", 0, 0, 0, 1, 0, 0))
	c.Read() // CharCreateOk
	c.Read() // CharSelectInfo
	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	c.Send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	c.Send(encodeAnswerTradeRequest(0))
	reply := c.Read()
	if reply[0] != serverpackets.OpcodeSendTradeDone {
		t.Fatalf("answer without request opcode = %#x, want SendTradeDone (%#x)", reply[0], serverpackets.OpcodeSendTradeDone)
	}
	reply = c.Read()
	assertStaticSystemMessageFrame(t, reply, serverpackets.SystemMessageTargetNotFound)

	c.Send(encodeTradeRequest(999999))
	c.Send(encodeAddTradeItem(0, 500, 1))
	c.Send(encodeTradeDone(1))
	c.Send(encodeSingleOpcode(clientpackets.OpcodeRequestSkillList))
	reply = c.Read()
	if reply[0] != serverpackets.OpcodeSkillList {
		t.Fatalf("post-trade-dispatch opcode = %#x, want SkillList (%#x)", reply[0], serverpackets.OpcodeSkillList)
	}
}

func newStartedDirectTradeFixture(t *testing.T) (*GameClientLink, *gamesql.ItemStore, *testsupport.FrameCapture, *testsupport.FrameCapture, *livePlayer, *livePlayer) {
	t.Helper()
	link, store, firstCap, secondCap, first, second := newDirectTradeFixture(t)
	link.handleTradeRequest(first, clientpackets.TradeRequest{ObjectID: second.ObjectID()})
	link.handleAnswerTradeRequest(second, clientpackets.AnswerTradeRequest{Response: 1})
	return link, store, firstCap, secondCap, first, second
}

func newDirectTradeFixture(t *testing.T) (*GameClientLink, *gamesql.ItemStore, *testsupport.FrameCapture, *testsupport.FrameCapture, *livePlayer, *livePlayer) {
	t.Helper()
	state := world.New()
	firstCap, secondCap := &testsupport.FrameCapture{}, &testsupport.FrameCapture{}
	first := newTestLivePlayer(t, 1, firstCap)
	first.Name = "TraderOne"
	second := newTestLivePlayer(t, 2, secondCap)
	second.Name = "TraderTwo"
	state.Spawn(first, 0, 0, 0, 0)
	state.AddPlayer(first)
	state.Spawn(second, 100, 0, 0, 0)
	state.AddPlayer(second)
	testsupport.ResetCapture(firstCap, secondCap)

	store := gamesql.NewItemStore(sqltest.SharedDB(t))
	ids := &sequentialIDs{next: 1000}
	updates := task.NewInventoryUpdates()
	link := &GameClientLink{
		world:            state,
		itemTemplates:    testItemTemplates(),
		items:            store,
		ids:              ids,
		inventory:        invops.NewService(ids),
		trades:           tradebook.NewBook(time.Now),
		inventoryUpdates: updates,
		log:              zerolog.Nop(),
	}
	for _, live := range []*livePlayer{first, second} {
		inv := live.Inventory()
		live := live
		inv.SetUpdateNotifier(func() {
			updates.Add(inv, live)
		})
	}
	return link, store, firstCap, secondCap, first, second
}

func seedLiveItem(t *testing.T, live *livePlayer, objectID, templateID int32, count int) *item.Instance {
	t.Helper()
	inst := &item.Instance{ObjectID: objectID, TemplateID: templateID, OwnerID: live.ObjectID(), Count: count, Location: item.LocationInventory}
	result, _ := live.Inventory().Add(inst)
	live.Inventory().DrainUpdates()
	return result
}

func assertTradeUpdateCount(t *testing.T, frame []byte, want int32) {
	t.Helper()
	if frame[0] != serverpackets.OpcodeTradeUpdate {
		t.Fatalf("TradeUpdate opcode = %#x, want %#x", frame[0], serverpackets.OpcodeTradeUpdate)
	}
	r := wire.NewReader(frame[1:])
	r.ReadUint16()
	r.ReadUint16()
	r.ReadUint16()
	r.ReadInt32()
	r.ReadInt32()
	if got := r.ReadInt32(); got != want {
		t.Fatalf("TradeUpdate count = %d, want %d", got, want)
	}
	if err := r.Err(); err != nil {
		t.Fatalf("read TradeUpdate: %v", err)
	}
}

func assertTradeDoneFrame(t *testing.T, frame []byte, success bool) {
	t.Helper()
	if frame[0] != serverpackets.OpcodeSendTradeDone {
		t.Fatalf("SendTradeDone opcode = %#x, want %#x", frame[0], serverpackets.OpcodeSendTradeDone)
	}
	r := wire.NewReader(frame[1:])
	got := r.ReadInt32()
	want := int32(0)
	if success {
		want = 1
	}
	if got != want {
		t.Fatalf("SendTradeDone success = %d, want %d", got, want)
	}
	if err := r.Err(); err != nil {
		t.Fatalf("read SendTradeDone: %v", err)
	}
}

func encodeTradeRequest(objectID int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeTradeRequest)
	w.WriteInt32(objectID)
	return w.Bytes()
}

func encodeAnswerTradeRequest(response int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeAnswerTradeRequest)
	w.WriteInt32(response)
	return w.Bytes()
}

func encodeAddTradeItem(tradeID, objectID, count int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeAddTradeItem)
	w.WriteInt32(tradeID)
	w.WriteInt32(objectID)
	w.WriteInt32(count)
	return w.Bytes()
}

func encodeTradeDone(response int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeTradeDone)
	w.WriteInt32(response)
	return w.Bytes()
}
