package trade

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/trade"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// The fixture's soulshot and spiritshot: with no weapon held, enabling
// either is a grade mismatch that still turns auto use on.
const (
	soulshot          = 1463
	spiritshotNoGrade = 2509
)

func encodeRequestAutoSoulShot(itemID, typ int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeExtended)
	w.WriteUint16(clientpackets.OpcodeRequestAutoSoulShot)
	w.WriteInt32(itemID)
	w.WriteInt32(typ)
	return w.Bytes()
}

func encodeRequestSocialAction(actionID int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestSocialAction)
	w.WriteInt32(actionID)
	return w.Bytes()
}

func autoShotOn(t *testing.T, srv *gameservertest.Server, objID, itemID int32) bool {
	t.Helper()
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatalf("player %d not online", objID)
	}
	holder, ok := obj.(interface{ AutoSoulShotEnabled(int32) bool })
	if !ok {
		t.Fatalf("player %d = %T has no auto-shot state", objID, obj)
	}
	return holder.AutoSoulShotEnabled(itemID)
}

// assertAutoShotToggled reads the ExAutoSoulShot a toggle answers with and
// drains the notices after it.
func assertAutoShotToggled(t *testing.T, c *testsupport.ScriptedClient, itemID int32, enabled bool) {
	t.Helper()
	frame := c.Read()
	assertFrameOpcode(t, frame, serverpackets.OpcodeExtended, "ExAutoSoulShot")
	r := wire.NewReader(frame[1:])
	if sub := r.ReadUint16(); sub != serverpackets.OpcodeExAutoSoulShot {
		t.Fatalf("extended opcode = %#x, want ExAutoSoulShot (%#x)", sub, serverpackets.OpcodeExAutoSoulShot)
	}
	want := int32(0)
	if enabled {
		want = 1
	}
	if gotItem, gotOn := r.ReadInt32(), r.ReadInt32(); gotItem != itemID || gotOn != want {
		t.Fatalf("ExAutoSoulShot = item %d on %d, want item %d on %d", gotItem, gotOn, itemID, want)
	}
	drainUntilQuiet(t, c)
}

// assertSocialAction reads a SocialAction showing actorID's emote.
func assertSocialAction(t *testing.T, c *testsupport.ScriptedClient, actorID, actionID int32, what string) {
	t.Helper()
	frame := c.Read()
	assertFrameOpcode(t, frame, serverpackets.OpcodeSocialAction, what)
	r := wire.NewReader(frame[1:])
	if gotActor, gotAction := r.ReadInt32(), r.ReadInt32(); gotActor != actorID || gotAction != actionID {
		t.Fatalf("%s = actor %d action %d, want actor %d action %d", what, gotActor, gotAction, actorID, actionID)
	}
}

// sendTradeRequestToSecond has the first trader ask the second to trade and
// consumes the request's frames, leaving the second holding an unanswered
// request.
func (h *traders) sendTradeRequestToSecond(t *testing.T) {
	t.Helper()
	h.first.Send(encodeTradeRequest(h.secondID))
	assertFrameOpcode(t, h.second.Read(), serverpackets.OpcodeSendTradeRequest, "SendTradeRequest")
	assertSystemMessageText(t, h.first.Read(), serverpackets.SystemMessageRequestS1ForTrade, "TraderTwo")
}

func manualTradeClock() (func() time.Time, func(time.Duration)) {
	var clock atomic.Int64
	clock.Store(time.Unix(1_000_000, 0).UnixNano())
	return func() time.Time { return time.Unix(0, clock.Load()) },
		func(d time.Duration) { clock.Add(int64(d)) }
}

// TestAutoSoulShotIgnoredWhileHoldingRequest pins RequestAutoSoulShot.java:28:
// while Player.getActiveRequester() is set, the packet is dropped with no
// reply, enabling or disabling, and auto use stays as it was. The requester
// itself is not held. Once the request expires (Player.java:3030) the same
// toggles go through.
func TestAutoSoulShotIgnoredWhileHoldingRequest(t *testing.T) {
	t.Parallel()
	now, advance := manualTradeClock()
	h := bootTraders(t, gameservertest.WithTradeClock(now))
	for _, id := range []int32{h.firstID, h.secondID} {
		h.srv.GiveItem(t, id, soulshot, 10)
		h.srv.GiveItem(t, id, spiritshotNoGrade, 10)
	}
	h.enterAll(t)

	h.second.Send(encodeRequestAutoSoulShot(spiritshotNoGrade, 1))
	assertAutoShotToggled(t, h.second, spiritshotNoGrade, true)

	h.sendTradeRequestToSecond(t)

	h.second.Send(encodeRequestAutoSoulShot(soulshot, 1))
	assertSilent(t, h.second, "enable while holding a trade request")
	if autoShotOn(t, h.srv, h.secondID, soulshot) {
		t.Fatal("enable while holding a trade request turned auto use on")
	}
	h.second.Send(encodeRequestAutoSoulShot(spiritshotNoGrade, 0))
	assertSilent(t, h.second, "disable while holding a trade request")
	if !autoShotOn(t, h.srv, h.secondID, spiritshotNoGrade) {
		t.Fatal("disable while holding a trade request turned auto use off")
	}

	h.first.Send(encodeRequestAutoSoulShot(soulshot, 1))
	assertAutoShotToggled(t, h.first, soulshot, true)
	if !autoShotOn(t, h.srv, h.firstID, soulshot) {
		t.Fatal("the requester's own enable was ignored")
	}

	advance(trade.RequestTimeout)

	h.second.Send(encodeRequestAutoSoulShot(soulshot, 1))
	assertAutoShotToggled(t, h.second, soulshot, true)
	h.second.Send(encodeRequestAutoSoulShot(spiritshotNoGrade, 0))
	assertAutoShotToggled(t, h.second, spiritshotNoGrade, false)
	if !autoShotOn(t, h.srv, h.secondID, soulshot) || autoShotOn(t, h.srv, h.secondID, spiritshotNoGrade) {
		t.Fatal("toggles after the request expired did not apply")
	}
}

// TestAutoSoulShotIgnoredWhileTradingOnExpiredInvite pins the
// _activeTradeList clause of Player.getActiveRequester (Player.java:3030): a
// party invitation received with a trade window open keeps holding its
// target past its expiry while the window stays open, so RequestAutoSoulShot
// is still dropped.
func TestAutoSoulShotIgnoredWhileTradingOnExpiredInvite(t *testing.T) {
	t.Parallel()
	now, advance := manualTradeClock()
	h := bootTraders(t, gameservertest.WithTradeClock(now))
	h.srv.GiveItem(t, h.secondID, soulshot, 10)
	h.enterAll(t)
	inviter, _ := h.third(t, "player3", "Inviter")
	h.startTrade(t)

	w := wire.NewPacketWriter(clientpackets.OpcodeRequestJoinParty)
	w.WriteString("TraderTwo")
	w.WriteInt32(0)
	inviter.Send(w.Bytes())
	assertSystemMessageText(t, inviter.Read(), serverpackets.SystemMessageYouInvitedS1ToParty, "TraderTwo")
	assertFrameOpcode(t, h.second.Read(), serverpackets.OpcodeAskJoinParty, "AskJoinParty")
	advance(trade.RequestTimeout + time.Second)

	h.second.Send(encodeRequestAutoSoulShot(soulshot, 1))
	assertSilent(t, h.second, "enable while trading on an expired invitation")
	if autoShotOn(t, h.srv, h.secondID, soulshot) {
		t.Fatal("enable while trading on an expired invitation turned auto use on")
	}
}

// TestSocialActionIgnoredWhileHoldingRequest pins RequestSocialAction.java:38:
// a player holding an unanswered request shows no emote, to itself or anyone
// else; the requester still can, and so can the target once the request
// expires.
func TestSocialActionIgnoredWhileHoldingRequest(t *testing.T) {
	t.Parallel()
	now, advance := manualTradeClock()
	h := bootTraders(t, gameservertest.WithTradeClock(now))
	h.enterAll(t)
	h.sendTradeRequestToSecond(t)

	h.second.Send(encodeRequestSocialAction(2))
	assertSilent(t, h.second, "emote while holding a trade request")
	assertSilent(t, h.first, "watcher of an emote while holding a trade request")

	h.first.Send(encodeRequestSocialAction(3))
	assertSocialAction(t, h.first, h.firstID, 3, "requester's own emote")
	assertSocialAction(t, h.second, h.firstID, 3, "requester's emote seen by the target")

	advance(trade.RequestTimeout)

	h.second.Send(encodeRequestSocialAction(2))
	assertSocialAction(t, h.second, h.secondID, 2, "emote after the request expired")
	assertSocialAction(t, h.first, h.secondID, 2, "emote after expiry seen by the requester")
}
