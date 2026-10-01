package trade

import (
	"context"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

const silenceWindow = 300 * time.Millisecond

// TestTeleportCancelsOpenTrade pins Player.teleportTo: an accepted teleport
// cancels the teleported player's open trade after the jump, so both
// clients get SendTradeDone failure plus the canceled-trade message naming
// the one who moved, and a later add-item from the partner is swallowed.
func TestTeleportCancelsOpenTrade(t *testing.T) {
	t.Parallel()
	h := bootTraders(t)
	adena := h.srv.GiveItem(t, h.secondID, item.AdenaID, 100)
	h.enterAll(t)
	h.startTrade(t)

	obj, ok := h.srv.State.Player(h.firstID)
	if !ok {
		t.Fatal("first trader missing from world state")
	}
	character, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("player %T is not an online character", obj)
	}
	character.TeleportTo(spawnX+20_000, spawnY, spawnZ, 0)

	for _, who := range []struct {
		name   string
		client *testsupport.ScriptedClient
	}{
		{"teleported", h.first},
		{"partner", h.second},
	} {
		frames := drainFrames(t, who.client)
		teleport := indexOfOpcode(frames, serverpackets.OpcodeTeleportToLocation)
		done := indexOfOpcode(frames, serverpackets.OpcodeSendTradeDone)
		if teleport < 0 || done < 0 || done < teleport {
			t.Fatalf("%s frames = %x, want TeleportToLocation then SendTradeDone", who.name, opcodes(frames))
		}
		if tradeDones(frames) != 1 {
			t.Fatalf("%s frames = %x, want exactly one SendTradeDone", who.name, opcodes(frames))
		}
		if got := wire.NewReader(frames[done][1:]).ReadInt32(); got != 0 {
			t.Fatalf("%s SendTradeDone success = %d, want 0", who.name, got)
		}
		if done+1 >= len(frames) {
			t.Fatalf("%s frames = %x, want the canceled-trade message after SendTradeDone", who.name, opcodes(frames))
		}
		assertSystemMessageText(t, frames[done+1], serverpackets.SystemMessageS1CanceledTrade, "TraderOne")
	}

	h.second.Send(encodeAddTradeItem(0, adena, 10))
	assertSilent(t, h.second, "partner add after teleport cancel")
	assertSilent(t, h.first, "teleported trader after partner's late add")
}

// TestLogoutLeavesPartnerWindowOpen pins Player.deleteMe with an open trade
// window: the answered request no longer holds an active requester, so the
// exit cancels nothing and the partner hears nothing. The partner learns of
// it on its next add-item: TARGET_IS_NOT_FOUND_IN_THE_GAME, then the cancel
// pair naming the partner itself, and the session is gone.
func TestLogoutLeavesPartnerWindowOpen(t *testing.T) {
	t.Parallel()
	h := bootTraders(t)
	adena := h.srv.GiveItem(t, h.firstID, item.AdenaID, 100)
	h.enterAll(t)
	h.startTrade(t)

	h.second.Send(encodeSingleOpcode(clientpackets.OpcodeLogout))
	h.awaitOffline(t, h.secondID)
	assertNoTradeClose(t, drainFrames(t, h.first), "partner at logout")

	h.first.Send(encodeAddTradeItem(0, adena, 10))
	assertStaticSystemMessage(t, h.first.Read(), serverpackets.SystemMessageTargetNotFound)
	assertCancelPair(t, h.first, "TraderOne")
	assertSilent(t, h.first, "partner after the deferred cancel")

	h.first.Send(encodeAddTradeItem(0, adena, 10))
	assertSilent(t, h.first, "add after the deferred cancel")
}

// TestLogoutPartnerConfirmIsTargetNotFound pins TradeDone(1) against a
// partner who logged out with the window open: TARGET_IS_NOT_FOUND_IN_THE_GAME
// alone, the window stays open, and TradeDone(0) then cancels naming the
// remaining trader.
func TestLogoutPartnerConfirmIsTargetNotFound(t *testing.T) {
	t.Parallel()
	h := bootTraders(t)
	h.enterAll(t)
	h.startTrade(t)

	h.second.Send(encodeSingleOpcode(clientpackets.OpcodeLogout))
	h.awaitOffline(t, h.secondID)
	drainUntilQuiet(t, h.first)

	h.first.Send(encodeTradeDone(1))
	assertStaticSystemMessage(t, h.first.Read(), serverpackets.SystemMessageTargetNotFound)
	assertSilent(t, h.first, "confirm against a departed partner")

	h.first.Send(encodeTradeDone(0))
	assertCancelPair(t, h.first, "TraderOne")
	assertSilent(t, h.first, "cancel against a departed partner")
}

// TestRelogDoesNotRejoinDepartedTrade restarts the partner out of an open
// trade and back into the world under the same object id. The new login is
// free of the old session: it can request a fresh trade, and nothing the
// remaining trader does reaches it, not even ending the old session, which
// leaves the new login's own trade open. Against the old session the remaining
// trader's add-item still goes through its own side (the reference's
// presence check finds the id online), while its confirm fails the
// departed partner's re-check and cancels naming itself. Nothing the
// departed side offered moves.
func TestRelogDoesNotRejoinDepartedTrade(t *testing.T) {
	t.Parallel()
	// No character-select reuse delay, so the restart can select again at once.
	h := bootTraders(t, gameservertest.WithReuseDelays(0, 0))
	firstAdena := h.srv.GiveItem(t, h.firstID, item.AdenaID, 100)
	secondAdena := h.srv.GiveItem(t, h.secondID, item.AdenaID, 200)
	h.enterAll(t)
	h.startTrade(t)

	h.second.Send(encodeAddTradeItem(0, secondAdena, 50))
	drainUntilQuiet(t, h.second)
	drainUntilQuiet(t, h.first)

	h.second.Send(encodeSingleOpcode(clientpackets.OpcodeRequestRestart))
	assertFrameOpcode(t, h.second.Read(), serverpackets.OpcodeRestartResponse, "RestartResponse")
	h.awaitOffline(t, h.secondID)
	assertNoTradeClose(t, drainFrames(t, h.first), "partner at restart")
	drainUntilQuiet(t, h.second)
	startInWorld(t, h.second)
	drainUntilQuiet(t, h.first)

	// The new login opens a trade of its own before the old session ends.
	third, thirdID := h.third(t, "player3", "TraderThree")
	h.second.Send(encodeTradeRequest(thirdID))
	assertFrameOpcode(t, third.Read(), serverpackets.OpcodeSendTradeRequest, "SendTradeRequest")
	assertSystemMessageText(t, h.second.Read(), serverpackets.SystemMessageRequestS1ForTrade, "TraderThree")
	third.Send(encodeAnswerTradeRequest(1))
	drainUntilQuiet(t, third)
	drainUntilQuiet(t, h.second)

	h.first.Send(encodeAddTradeItem(0, firstAdena, 10))
	assertOwnOfferFrames(t, h.first.Read(), h.first.Read(), h.first.Read(), firstAdena, item.AdenaID, 10, 90)
	assertSilent(t, h.first, "own add against a relogged partner")
	assertSilent(t, h.second, "relogged partner after the old session's add")

	h.first.Send(encodeTradeDone(1))
	assertCancelPair(t, h.first, "TraderOne")
	assertSilent(t, h.first, "confirm against a relogged partner")
	assertSilent(t, h.second, "relogged partner after the old session's cancel")

	// Ending the old session left the new login's own trade open.
	h.second.Send(encodeAddTradeItem(0, secondAdena, 30))
	assertOwnOfferFrames(t, h.second.Read(), h.second.Read(), h.second.Read(), secondAdena, item.AdenaID, 30, 170)
	assertTradeAddRow(t, third.Read(), serverpackets.OpcodeTradeOtherAdd, item.AdenaID, 30)

	h.srv.FlushItems(t)
	for _, want := range []struct {
		owner, object int32
		count         int
	}{
		{h.firstID, firstAdena, 100},
		{h.secondID, secondAdena, 200},
	} {
		rows, err := h.srv.Items.ListByOwner(context.Background(), want.owner)
		if err != nil {
			t.Fatalf("list items of %d: %v", want.owner, err)
		}
		if len(rows) != 1 || rows[0].ObjectID != want.object || rows[0].Count != want.count {
			t.Fatalf("persisted rows of %d = %+v, want object %d count %d", want.owner, rows, want.object, want.count)
		}
	}
}

func encodeSingleOpcode(opcode byte) []byte {
	return wire.NewPacketWriter(opcode).Bytes()
}

func (h *traders) awaitOffline(t *testing.T, objID int32) {
	t.Helper()
	h.srv.AdvanceUntil(t, "player leaves the world", func() bool {
		_, ok := h.srv.State.Player(objID)
		return !ok
	})
}

func assertCancelPair(t *testing.T, c *testsupport.ScriptedClient, canceller string) {
	t.Helper()
	frame := c.Read()
	assertFrameOpcode(t, frame, serverpackets.OpcodeSendTradeDone, "SendTradeDone")
	if got := wire.NewReader(frame[1:]).ReadInt32(); got != 0 {
		t.Fatalf("SendTradeDone success = %d, want 0", got)
	}
	assertSystemMessageText(t, c.Read(), serverpackets.SystemMessageS1CanceledTrade, canceller)
}

func assertNoTradeClose(t *testing.T, frames [][]byte, what string) {
	t.Helper()
	if tradeDones(frames) != 0 {
		t.Fatalf("%s received %x, want no SendTradeDone", what, opcodes(frames))
	}
	for _, f := range frames {
		if f[0] == serverpackets.OpcodeSystemMessage && wire.NewReader(f[1:]).ReadInt32() == serverpackets.SystemMessageS1CanceledTrade {
			t.Fatalf("%s received the canceled-trade message", what)
		}
	}
}

func assertSilent(t *testing.T, c *testsupport.ScriptedClient, what string) {
	t.Helper()
	if frame := c.ReadWithTimeout(silenceWindow); frame != nil {
		t.Fatalf("%s: received %#x, want silence", what, frame[0])
	}
}

func indexOfOpcode(frames [][]byte, opcode byte) int {
	for i, f := range frames {
		if f[0] == opcode {
			return i
		}
	}
	return -1
}

func tradeDones(frames [][]byte) int {
	n := 0
	for _, f := range frames {
		if f[0] == serverpackets.OpcodeSendTradeDone {
			n++
		}
	}
	return n
}

func opcodes(frames [][]byte) []byte {
	out := make([]byte, 0, len(frames))
	for _, f := range frames {
		out = append(out, f[0])
	}
	return out
}
