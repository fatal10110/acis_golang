//go:build integration

package network

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestSettleConfirmedTradeOutOfRangeCancels pins the settlement re-validate:
// when the pair no longer passes liveness/distance at exchange time, the
// reference cancels the whole trade for both players (SendTradeDone failure
// plus canceled-trade messages), matching every other cancel path — it does
// not finish the trade with the exchange-ended transfer-failure messages.
func TestSettleConfirmedTradeOutOfRangeCancels(t *testing.T) {
	link, _, firstCap, secondCap, first, second := newDirectTradeFixture(t)
	ctx := context.Background()

	link.handleTradeRequest(first, clientpackets.TradeRequest{ObjectID: second.ObjectID()})
	link.handleAnswerTradeRequest(second, clientpackets.AnswerTradeRequest{Response: 1})
	session, ok := link.trades.Session(first.ObjectID())
	if !ok {
		t.Fatal("trade session missing after handshake")
	}
	link.handleTradeDone(ctx, first, clientpackets.TradeDone{Response: 1})

	if err := link.world.Move(second, 1000, 0, 0); err != nil {
		t.Fatalf("Move: %v", err)
	}
	testsupport.ResetCapture(firstCap, secondCap)

	link.settleConfirmedTrade(ctx, session, second.ObjectID())

	testsupport.AssertOpcodeSequence(t, firstCap.Frames(),
		serverpackets.OpcodeSendTradeDone, serverpackets.OpcodeSystemMessage)
	assertTradeDoneFrame(t, firstCap.Frames()[0], false)
	assertSystemMessageStringFrame(t, firstCap.Frames()[1], serverpackets.SystemMessageS1CanceledTrade, "TraderTwo")
	testsupport.AssertOpcodeSequence(t, secondCap.Frames(),
		serverpackets.OpcodeSendTradeDone, serverpackets.OpcodeSystemMessage)
	assertTradeDoneFrame(t, secondCap.Frames()[0], false)
	assertSystemMessageStringFrame(t, secondCap.Frames()[1], serverpackets.SystemMessageS1CanceledTrade, "TraderOne")

	if link.trades.HasActive(first.ObjectID()) || link.trades.HasActive(second.ObjectID()) {
		t.Fatal("trade session was not cleared after failed settlement re-validation")
	}
}
