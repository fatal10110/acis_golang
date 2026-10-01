package trade

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestTargetRelogDropsPendingRequest pins Player.deleteMe clearing the active
// requester: the target restarts with a request pending and comes back under
// the same id, and its accept finds nothing to answer — SendTradeDone(0) and
// TARGET_IS_NOT_FOUND_IN_THE_GAME, no trade on either side. The requester's
// own request is untouched by the target's exit, so it stays busy until the
// request expires.
func TestTargetRelogDropsPendingRequest(t *testing.T) {
	t.Parallel()
	// No character-select reuse delay, so the restart can select again at once.
	h := bootTraders(t, gameservertest.WithReuseDelays(0, 0))
	h.enterAll(t)
	h.sendRequest(t)

	h.relog(t, h.second, h.secondID)

	h.second.Send(encodeAnswerTradeRequest(1))
	frame := h.second.Read()
	assertFrameOpcode(t, frame, serverpackets.OpcodeSendTradeDone, "SendTradeDone")
	if got := wire.NewReader(frame[1:]).ReadInt32(); got != 0 {
		t.Fatalf("SendTradeDone success = %d, want 0", got)
	}
	assertStaticSystemMessage(t, h.second.Read(), serverpackets.SystemMessageTargetNotFound)
	assertSilent(t, h.second, "relogged target after its answer")
	assertSilent(t, h.first, "requester after the relogged target's answer")

	h.first.Send(encodeTradeRequest(h.secondID))
	assertStaticSystemMessage(t, h.first.Read(), serverpackets.SystemMessageAlreadyTrading)
	assertSilent(t, h.second, "relogged target after the busy requester's retry")
}

// TestRequesterRelogLeavesRequestBehind pins a requester that restarts with
// its request pending and comes back under the same id. The new login never
// asked: it is free to trade at once, and the target's answer goes to the
// login that asked. Accepting opens the window on the target alone, against
// a partner that is gone — its own add goes through on its side and its
// cancel names itself — while the new login hears none of it and its own
// request carries on.
func TestRequesterRelogLeavesRequestBehind(t *testing.T) {
	t.Parallel()
	h := bootTraders(t, gameservertest.WithReuseDelays(0, 0))
	adena := h.srv.GiveItem(t, h.secondID, item.AdenaID, 100)
	h.enterAll(t)
	h.sendRequest(t)

	h.relog(t, h.first, h.firstID)
	third, thirdID := h.third(t, "player3", "TraderThree")
	h.first.Send(encodeTradeRequest(thirdID))
	assertFrameOpcode(t, third.Read(), serverpackets.OpcodeSendTradeRequest, "SendTradeRequest")
	assertSystemMessageText(t, h.first.Read(), serverpackets.SystemMessageRequestS1ForTrade, "TraderThree")

	h.second.Send(encodeAnswerTradeRequest(1))
	assertSystemMessageText(t, h.second.Read(), serverpackets.SystemMessageBeginTradeWithS1, "TraderOne")
	frame := h.second.Read()
	assertFrameOpcode(t, frame, serverpackets.OpcodeTradeStart, "TradeStart")
	if got := wire.NewReader(frame[1:]).ReadInt32(); got != h.firstID {
		t.Fatalf("TradeStart partner id = %d, want %d", got, h.firstID)
	}
	assertSilent(t, h.second, "target after accepting")
	assertSilent(t, h.first, "relogged requester after the old request's accept")

	h.second.Send(encodeAddTradeItem(0, adena, 10))
	assertOwnOfferFrames(t, h.second.Read(), h.second.Read(), h.second.Read(), adena, item.AdenaID, 10, 90)
	assertSilent(t, h.second, "target's own add against the departed requester")
	assertSilent(t, h.first, "relogged requester after the target's add")

	h.second.Send(encodeTradeDone(0))
	assertCancelPair(t, h.second, "TraderTwo")
	assertSilent(t, h.first, "relogged requester after the target's cancel")

	third.Send(encodeAnswerTradeRequest(1))
	assertSystemMessageText(t, h.first.Read(), serverpackets.SystemMessageBeginTradeWithS1, "TraderThree")
	assertFrameOpcode(t, h.first.Read(), serverpackets.OpcodeTradeStart, "new login's TradeStart")
}

// TestRequesterRelogDenialReachesNoOne pins a denial of a request whose
// requester restarted: the denial belongs to the login that asked, so the
// new login under its id hears nothing, and neither does the target.
func TestRequesterRelogDenialReachesNoOne(t *testing.T) {
	t.Parallel()
	h := bootTraders(t, gameservertest.WithReuseDelays(0, 0))
	h.enterAll(t)
	h.sendRequest(t)

	h.relog(t, h.first, h.firstID)

	h.second.Send(encodeAnswerTradeRequest(0))
	assertSilent(t, h.first, "relogged requester after the old request's denial")
	assertSilent(t, h.second, "target after denying")
}

// sendRequest has the first trader ask the second and consumes the
// request's frames, leaving the request pending.
func (h *traders) sendRequest(t *testing.T) {
	t.Helper()
	h.first.Send(encodeTradeRequest(h.secondID))
	assertFrameOpcode(t, h.second.Read(), serverpackets.OpcodeSendTradeRequest, "SendTradeRequest")
	assertSystemMessageText(t, h.first.Read(), serverpackets.SystemMessageRequestS1ForTrade, "TraderTwo")
}

// relog restarts c's character out of the world and selects it again under
// the same object id, then drains both traders.
func (h *traders) relog(t *testing.T, c *testsupport.ScriptedClient, objID int32) {
	t.Helper()
	c.Send(encodeSingleOpcode(clientpackets.OpcodeRequestRestart))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeRestartResponse, "RestartResponse")
	h.awaitOffline(t, objID)
	drainUntilQuiet(t, c)
	startInWorld(t, c)
	drainUntilQuiet(t, h.first)
	drainUntilQuiet(t, h.second)
}
