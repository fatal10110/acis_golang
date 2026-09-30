package network

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// openDirectTrade opens a trade between first and second and clears the
// frames the opening sent.
func openDirectTrade(t *testing.T, link *GameClientLink, first, second *livePlayer, caps ...*testsupport.FrameCapture) {
	t.Helper()
	link.handleTradeRequest(first, clientpackets.TradeRequest{ObjectID: second.ObjectID()})
	link.handleAnswerTradeRequest(second, clientpackets.AnswerTradeRequest{Response: 1})
	if !link.trades.HasActive(first.ObjectID()) || !link.trades.HasActive(second.ObjectID()) {
		t.Fatal("trade did not open")
	}
	testsupport.ResetCapture(caps...)
}

// TestAddTradeItemWithoutTransactionRightCancelsTrade pins the AddTradeItem
// access gate: a trader whose access level forbids transactions is told it
// is not authorized and the whole trade is cancelled for both sides.
func TestAddTradeItemWithoutTransactionRightCancelsTrade(t *testing.T) {
	link, _, firstCap, secondCap, first, second := newDirectTradeFixture(t)
	openDirectTrade(t, link, first, second, firstCap, secondCap)

	first.access.AllowTransaction = false
	link.handleAddTradeItem(first, clientpackets.AddTradeItem{ObjectID: 12345, Count: 1})

	testsupport.AssertOpcodeSequence(t, firstCap.Frames(),
		serverpackets.OpcodeSystemMessage, serverpackets.OpcodeSendTradeDone, serverpackets.OpcodeSystemMessage)
	assertSystemMessageIDFrame(t, firstCap.Frames()[0], serverpackets.SystemMessageNotAuthorizedToDoThat)
	assertTradeDoneFrame(t, firstCap.Frames()[1], false)
	assertSystemMessageStringFrame(t, firstCap.Frames()[2], serverpackets.SystemMessageS1CanceledTrade, "TraderOne")
	testsupport.AssertOpcodeSequence(t, secondCap.Frames(),
		serverpackets.OpcodeSendTradeDone, serverpackets.OpcodeSystemMessage)
	assertTradeDoneFrame(t, secondCap.Frames()[0], false)
	assertSystemMessageStringFrame(t, secondCap.Frames()[1], serverpackets.SystemMessageS1CanceledTrade, "TraderOne")

	if link.trades.HasActive(first.ObjectID()) || link.trades.HasActive(second.ObjectID()) {
		t.Fatal("trade session still active after the access gate cancelled it")
	}
}

// TestTradeDoneWithoutTransactionRightKeepsTradeOpen pins the TradeDone
// access gate: unlike AddTradeItem it only answers not-authorized and leaves
// the trade open and unconfirmed, so the partner's later confirm is the
// first confirmation and does not settle anything.
func TestTradeDoneWithoutTransactionRightKeepsTradeOpen(t *testing.T) {
	link, _, firstCap, secondCap, first, second := newDirectTradeFixture(t)
	ctx := context.Background()
	openDirectTrade(t, link, first, second, firstCap, secondCap)

	first.access.AllowTransaction = false
	link.handleTradeDone(ctx, first, clientpackets.TradeDone{Response: 1})

	testsupport.AssertOpcodeSequence(t, firstCap.Frames(), serverpackets.OpcodeSystemMessage)
	assertSystemMessageIDFrame(t, firstCap.Frames()[0], serverpackets.SystemMessageNotAuthorizedToDoThat)
	if n := len(secondCap.Frames()); n != 0 {
		t.Fatalf("partner received %d frames, want none", n)
	}
	if !link.trades.HasActive(first.ObjectID()) || !link.trades.HasActive(second.ObjectID()) {
		t.Fatal("access gate on TradeDone closed the trade")
	}

	// The refused confirm recorded nothing: the partner's confirm is only
	// the first side confirming, not a settle.
	testsupport.ResetCapture(firstCap, secondCap)
	link.handleTradeDone(ctx, second, clientpackets.TradeDone{Response: 1})

	testsupport.AssertOpcodeSequence(t, secondCap.Frames(), serverpackets.OpcodeTradePressOwnOk)
	testsupport.AssertOpcodeSequence(t, firstCap.Frames(),
		serverpackets.OpcodeSystemMessage, serverpackets.OpcodeTradePressOtherOk)
	assertSystemMessageStringFrame(t, firstCap.Frames()[0], serverpackets.SystemMessageS1ConfirmedTrade, "TraderTwo")
	if !link.trades.HasActive(first.ObjectID()) {
		t.Fatal("trade settled although the gated side never confirmed")
	}
}

// TestTradeDoneWithoutTransactionRightAfterPartnerLeftKeepsTradeOpen pins
// the TradeDone access gate on the departed-partner path: the partner left
// the world with the window open and its id resolves again, so the presence
// check passes and the access check answers next, before any confirm. The
// restricted confirmer is told it is not authorized and the trade stays open
// instead of being cancelled by the confirm's re-check.
func TestTradeDoneWithoutTransactionRightAfterPartnerLeftKeepsTradeOpen(t *testing.T) {
	link, _, firstCap, secondCap, first, second := newDirectTradeFixture(t)
	openDirectTrade(t, link, first, second, firstCap, secondCap)

	link.leaveActiveTrade(second)
	testsupport.ResetCapture(firstCap, secondCap)
	first.access.AllowTransaction = false
	link.handleTradeDone(context.Background(), first, clientpackets.TradeDone{Response: 1})

	testsupport.AssertOpcodeSequence(t, firstCap.Frames(), serverpackets.OpcodeSystemMessage)
	assertSystemMessageIDFrame(t, firstCap.Frames()[0], serverpackets.SystemMessageNotAuthorizedToDoThat)
	if n := len(secondCap.Frames()); n != 0 {
		t.Fatalf("partner received %d frames, want none", n)
	}
	if !link.trades.HasActive(first.ObjectID()) {
		t.Fatal("access gate on TradeDone closed the trade on the departed-partner path")
	}
}
