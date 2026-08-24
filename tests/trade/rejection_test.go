package trade

import (
	"context"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestTradeRequestRejectsIncorrectTargets pins the request-side target gates:
// trading with yourself answers TARGET_IS_INCORRECT, an unknown object id is
// answered with silence exactly like the reference (no pending client action
// is registered by a request whose target does not exist), and a valid
// request still works afterwards.
func TestTradeRequestRejectsIncorrectTargets(t *testing.T) {
	h := bootTraders(t)
	h.enterAll(t)

	h.first.Send(encodeTradeRequest(h.firstID))
	assertStaticSystemMessage(t, h.first.Read(), serverpackets.SystemMessageTargetIncorrect)
	drainUntilQuiet(t, h.second)

	h.first.Send(encodeTradeRequest(999999))
	if frame := h.first.ReadWithTimeout(300 * time.Millisecond); frame != nil {
		t.Fatalf("unknown-target reply opcode = %#x, want silence", frame[0])
	}

	h.startTrade(t)
}

// TestTradeRequestRejectsBusyRequester pins the ALREADY_TRADING answer a
// client receives when it requests a second trade while its first request is
// still pending; the target receives nothing further.
func TestTradeRequestRejectsBusyRequester(t *testing.T) {
	h := bootTraders(t)
	h.enterAll(t)

	h.first.Send(encodeTradeRequest(h.secondID))
	assertFrameOpcode(t, h.second.Read(), serverpackets.OpcodeSendTradeRequest, "SendTradeRequest")
	assertSystemMessageText(t, h.first.Read(), serverpackets.SystemMessageRequestS1ForTrade, "TraderTwo")

	h.first.Send(encodeTradeRequest(h.secondID))
	assertStaticSystemMessage(t, h.first.Read(), serverpackets.SystemMessageAlreadyTrading)
	drainUntilQuiet(t, h.second)
}

// TestTradeRequestRejectsBusyTarget pins the S1_IS_BUSY_TRY_LATER answer a
// third player gets when requesting someone who already has a pending trade
// request from somebody else.
func TestTradeRequestRejectsBusyTarget(t *testing.T) {
	h := bootTraders(t)
	h.enterAll(t)
	third, _ := h.third(t, "player3", "TraderThree")

	h.first.Send(encodeTradeRequest(h.secondID))
	assertFrameOpcode(t, h.second.Read(), serverpackets.OpcodeSendTradeRequest, "SendTradeRequest")
	assertSystemMessageText(t, h.first.Read(), serverpackets.SystemMessageRequestS1ForTrade, "TraderTwo")

	third.Send(encodeTradeRequest(h.secondID))
	assertSystemMessageText(t, third.Read(), serverpackets.SystemMessageS1IsBusyTryLater, "TraderTwo")
	drainUntilQuiet(t, h.first)
	drainUntilQuiet(t, h.second)
}

// TestAddTradeItemRejectsUntradableItem pins the NothingHappened answer for
// offering an item the reference marks untradable: no offer packets on either
// side and no inventory mutation.
func TestAddTradeItemRejectsUntradableItem(t *testing.T) {
	h := bootTraders(t)
	shots := h.srv.GiveItem(t, h.firstID, 1463, 5) // soulshot templates are not tradable
	h.enterAll(t)
	h.startTrade(t)

	h.first.Send(encodeAddTradeItem(0, shots, 1))
	assertStaticSystemMessage(t, h.first.Read(), serverpackets.SystemMessageNothingHappened)
	if frame := h.second.ReadWithTimeout(300 * time.Millisecond); frame != nil {
		t.Fatalf("partner received opcode %#x for an untradable offer, want silence", frame[0])
	}

	h.srv.FlushItems(t)
	rows, err := h.srv.Items.ListByOwner(context.Background(), h.firstID)
	if err != nil {
		t.Fatalf("list items: %v", err)
	}
	for _, row := range rows {
		if row.ObjectID == shots && row.Count != 5 {
			t.Fatalf("soulshot count after rejected offer = %d, want 5", row.Count)
		}
	}
}

// TestConfirmOutOfRangeCancelsTradeForBoth pins the reference's confirm-time
// distance validation: confirming while the partner walked out of the
// interaction radius cancels the whole trade — both clients get
// SendTradeDone failure plus the canceled-trade message — and nothing is
// transferred.
func TestConfirmOutOfRangeCancelsTradeForBoth(t *testing.T) {
	h := bootTraders(t)
	adena := h.srv.GiveItem(t, h.firstID, item.AdenaID, 100)
	potions := h.srv.GiveItem(t, h.secondID, 20, 3)
	h.enterAll(t)
	h.startTrade(t)

	h.first.Send(encodeAddTradeItem(0, adena, 40))
	assertOwnOfferFrames(t, h.first.Read(), h.first.Read(), h.first.Read(), adena, item.AdenaID, 40, 60)
	drainUntilQuiet(t, h.second)

	h.first.Send(encodeTradeDone(1))
	assertFrameOpcode(t, h.first.Read(), serverpackets.OpcodeTradePressOwnOk, "first TradePressOwnOk")
	assertSystemMessageText(t, h.second.Read(), serverpackets.SystemMessageS1ConfirmedTrade, "TraderOne")
	assertFrameOpcode(t, h.second.Read(), serverpackets.OpcodeTradePressOtherOk, "second TradePressOtherOk")

	// The second trader walks far beyond the interaction radius before
	// confirming.
	const farX = spawnX + 2*tradeInteractionDistance
	h.second.Send(encodeMoveBackwardToLocation(farX, spawnY, spawnZ, spawnX, spawnY, spawnZ))
	assertFrameOpcode(t, h.second.Read(), serverpackets.OpcodeMoveToLocation, "second MoveToLocation")
	waitForArrival(t, h, h.secondID, farX)
	drainUntilQuiet(t, h.first)
	drainUntilQuiet(t, h.second)

	h.second.Send(encodeTradeDone(1))
	for _, who := range []struct {
		name   string
		client *testsupport.ScriptedClient
		text   string
	}{
		{"first", h.first, "TraderTwo"},
		{"second", h.second, "TraderOne"},
	} {
		frame := who.client.Read()
		assertFrameOpcode(t, frame, serverpackets.OpcodeSendTradeDone, who.name+" SendTradeDone")
		if got := wire.NewReader(frame[1:]).ReadInt32(); got != 0 {
			t.Fatalf("%s SendTradeDone success = %d, want 0", who.name, got)
		}
		assertSystemMessageText(t, who.client.Read(), serverpackets.SystemMessageS1CanceledTrade, who.text)
	}
	drainUntilQuiet(t, h.first)
	drainUntilQuiet(t, h.second)

	h.srv.FlushItems(t)
	ctx := context.Background()
	firstRows, err := h.srv.Items.ListByOwner(ctx, h.firstID)
	if err != nil {
		t.Fatalf("list first items: %v", err)
	}
	if len(firstRows) != 1 || firstRows[0].ObjectID != adena || firstRows[0].Count != 100 {
		t.Fatalf("persisted first rows after canceled trade = %+v, want adena count 100", firstRows)
	}
	secondRows, err := h.srv.Items.ListByOwner(ctx, h.secondID)
	if err != nil {
		t.Fatalf("list second items: %v", err)
	}
	if len(secondRows) != 1 || secondRows[0].ObjectID != potions || secondRows[0].Count != 3 {
		t.Fatalf("persisted second rows after canceled trade = %+v, want potion count 3", secondRows)
	}
}
