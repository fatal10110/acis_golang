package network

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	tradebook "github.com/fatal10110/acis_golang/internal/gameserver/trade"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestSettleConfirmedTradeOutOfRangeCancels pins the settlement re-validate:
// when the pair no longer passes liveness/distance at exchange time, the
// reference cancels the whole trade for both players (SendTradeDone failure
// plus canceled-trade messages naming the confirmer, who cancels), matching
// every other cancel path — it does not finish the trade with the
// exchange-ended transfer-failure messages.
func TestSettleConfirmedTradeOutOfRangeCancels(t *testing.T) {
	link, _, firstCap, secondCap, first, second := newDirectTradeFixture(t)
	ctx := context.Background()

	session := readyTradeSession(t, ctx, link, first, second)

	if err := link.world.Move(second, 1000, 0, 0); err != nil {
		t.Fatalf("Move: %v", err)
	}
	testsupport.ResetCapture(firstCap, secondCap)

	link.settleConfirmedTrade(session, second.ObjectID())

	testsupport.AssertOpcodeSequence(t, firstCap.Frames(),
		serverpackets.OpcodeSendTradeDone, serverpackets.OpcodeSystemMessage)
	assertTradeDoneFrame(t, firstCap.Frames()[0], false)
	assertSystemMessageStringFrame(t, firstCap.Frames()[1], serverpackets.SystemMessageS1CanceledTrade, "TraderTwo")
	testsupport.AssertOpcodeSequence(t, secondCap.Frames(),
		serverpackets.OpcodeSendTradeDone, serverpackets.OpcodeSystemMessage)
	assertTradeDoneFrame(t, secondCap.Frames()[0], false)
	assertSystemMessageStringFrame(t, secondCap.Frames()[1], serverpackets.SystemMessageS1CanceledTrade, "TraderTwo")

	if link.trades.HasActive(first.ObjectID()) || link.trades.HasActive(second.ObjectID()) {
		t.Fatal("trade session was not cleared after failed settlement re-validation")
	}
}

// TestSettleConfirmedTradePartnerGoneCancelsConfirmer pins the settle-time
// cancel when the partner left the world after Confirm took the ready
// session out of the book (its own detach cancel then found nothing to
// cancel): the confirmer still gets the reference's cancelActiveTrade pair,
// naming itself, instead of a trade window left open.
func TestSettleConfirmedTradePartnerGoneCancelsConfirmer(t *testing.T) {
	link, _, firstCap, secondCap, first, second := newDirectTradeFixture(t)
	session := readyTradeSession(t, context.Background(), link, first, second)

	link.world.RemovePlayer(second.ObjectID())
	testsupport.ResetCapture(firstCap, secondCap)

	link.settleConfirmedTrade(session, first.ObjectID())

	testsupport.AssertOpcodeSequence(t, firstCap.Frames(),
		serverpackets.OpcodeSendTradeDone, serverpackets.OpcodeSystemMessage)
	assertTradeDoneFrame(t, firstCap.Frames()[0], false)
	assertSystemMessageStringFrame(t, firstCap.Frames()[1], serverpackets.SystemMessageS1CanceledTrade, "TraderOne")
	if n := len(secondCap.Frames()); n != 0 {
		t.Fatalf("departed partner received %d frames, want none", n)
	}
}

// TestCancelTradePartnerGoneCancelsCanceller pins a cancel whose partner left
// the world while the session was still open: the canceller still gets the
// SendTradeDone failure plus the canceled-trade message naming itself, and
// the one who left gets nothing.
func TestCancelTradePartnerGoneCancelsCanceller(t *testing.T) {
	link, _, firstCap, secondCap, first, second := newDirectTradeFixture(t)
	link.handleTradeRequest(first, clientpackets.TradeRequest{ObjectID: second.ObjectID()})
	link.handleAnswerTradeRequest(second, clientpackets.AnswerTradeRequest{Response: 1})

	link.world.RemovePlayer(second.ObjectID())
	testsupport.ResetCapture(firstCap, secondCap)

	link.cancelTradeByID(first.ObjectID())

	testsupport.AssertOpcodeSequence(t, firstCap.Frames(),
		serverpackets.OpcodeSendTradeDone, serverpackets.OpcodeSystemMessage)
	assertTradeDoneFrame(t, firstCap.Frames()[0], false)
	assertSystemMessageStringFrame(t, firstCap.Frames()[1], serverpackets.SystemMessageS1CanceledTrade, "TraderOne")
	if n := len(secondCap.Frames()); n != 0 {
		t.Fatalf("departed partner received %d frames, want none", n)
	}
	if link.trades.HasActive(first.ObjectID()) {
		t.Fatal("trade session was not cleared after cancel")
	}
}

// readyTradeSession opens a trade between first and second and confirms it
// on both sides the way handleTradeDone does, returning the ready session
// Confirm has already taken out of the book.
func readyTradeSession(t *testing.T, ctx context.Context, link *GameClientLink, first, second *livePlayer) tradebook.Session {
	t.Helper()
	link.handleTradeRequest(first, clientpackets.TradeRequest{ObjectID: second.ObjectID()})
	link.handleAnswerTradeRequest(second, clientpackets.AnswerTradeRequest{Response: 1})
	link.handleTradeDone(ctx, second, clientpackets.TradeDone{Response: 1})
	ready := link.trades.Confirm(first.ObjectID())
	if ready.Status != tradebook.DoneReady {
		t.Fatalf("Confirm status = %v, want ready", ready.Status)
	}
	return ready.Session
}
