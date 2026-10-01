package trade

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// enchantWeaponD and enchantScrollD are the fixture's D-grade sword and the
// D-grade weapon enchant scroll.
const (
	enchantWeaponD = 30
	enchantScrollD = 955
)

func encodeUseItem(objectID int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeUseItem)
	w.WriteInt32(objectID)
	w.WriteInt32(0)
	return w.Bytes()
}

func encodeRequestEnchantItem(objectID int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestEnchantItem)
	w.WriteInt32(objectID)
	return w.Bytes()
}

// selectEnchantScroll uses scroll from the item window and consumes the
// selection prompt pair.
func selectEnchantScroll(t *testing.T, c *testsupport.ScriptedClient, scroll int32) {
	t.Helper()
	c.Send(encodeUseItem(scroll))
	assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageSelectItemToEnchant)
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeChooseInventoryItem, "ChooseInventoryItem")
}

func assertEnchantCancelled(t *testing.T, frame []byte) {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeEnchantResult, "EnchantResult")
	if got := wire.NewReader(frame[1:]).ReadInt32(); got != int32(serverpackets.EnchantResultCancelled) {
		t.Fatalf("EnchantResult = %d, want CANCELLED", got)
	}
}

// TestPendingTradeRequestRefusesEnchant pins the RequestEnchantItem
// transaction gate: a player tied up in a trade request gets
// CANNOT_ENCHANT_WHILE_STORE and EnchantResult(CANCELLED), keeps the
// scroll, and loses the selection.
func TestPendingTradeRequestRefusesEnchant(t *testing.T) {
	t.Parallel()
	h := bootTraders(t)
	weapon := h.srv.GiveItem(t, h.secondID, enchantWeaponD, 1)
	scroll := h.srv.GiveItem(t, h.secondID, enchantScrollD, 1)
	h.enterAll(t)
	selectEnchantScroll(t, h.second, scroll)

	h.first.Send(encodeTradeRequest(h.secondID))
	assertFrameOpcode(t, h.second.Read(), serverpackets.OpcodeSendTradeRequest, "SendTradeRequest")
	drainUntilQuiet(t, h.first)

	h.second.Send(encodeRequestEnchantItem(weapon))
	assertStaticSystemMessage(t, h.second.Read(), serverpackets.SystemMessageCannotEnchantWhileStore)
	assertEnchantCancelled(t, h.second.Read())
	if held := h.srv.PlayerInventory(t, h.secondID).ItemByObjectID(scroll); held == nil || held.Snapshot().Count != 1 {
		t.Fatalf("scroll after the refused enchant = %+v, want one still held", held)
	}
}

// TestTradeConfirmCancelsBothEnchantSelections pins TradeDone(1): the
// confirmer's scroll selection is cancelled, then the partner's, each
// with EnchantResult(CANCELLED) and ENCHANT_SCROLL_CANCELLED ahead of the
// confirm itself.
func TestTradeConfirmCancelsBothEnchantSelections(t *testing.T) {
	t.Parallel()
	h := bootTraders(t)
	firstScroll := h.srv.GiveItem(t, h.firstID, enchantScrollD, 1)
	secondScroll := h.srv.GiveItem(t, h.secondID, enchantScrollD, 1)
	h.enterAll(t)
	selectEnchantScroll(t, h.first, firstScroll)
	selectEnchantScroll(t, h.second, secondScroll)
	h.startTrade(t)

	h.first.Send(encodeTradeDone(1))
	for _, who := range []struct {
		name   string
		client *testsupport.ScriptedClient
	}{{"confirmer", h.first}, {"partner", h.second}} {
		assertEnchantCancelled(t, who.client.Read())
		assertStaticSystemMessage(t, who.client.Read(), serverpackets.SystemMessageEnchantScrollCancelled)
		if rest := drainFrames(t, who.client); len(rest) == 0 {
			t.Fatalf("%s got nothing after the enchant cancel, want the confirm notice", who.name)
		}
	}
}

// TestTradeConfirmKeepsReloggedPartnerEnchant pins TradeDone(1) against a
// partner who restarted with the window open and came back: the partner
// cancel reaches only the departed login, so the new login's scroll
// selection survives the remaining trader's confirm, silently.
func TestTradeConfirmKeepsReloggedPartnerEnchant(t *testing.T) {
	t.Parallel()
	h := bootTraders(t, gameservertest.WithReuseDelays(0, 0))
	weapon := h.srv.GiveItem(t, h.secondID, enchantWeaponD, 1)
	scroll := h.srv.GiveItem(t, h.secondID, enchantScrollD, 1)
	h.enterAll(t)
	h.startTrade(t)

	h.second.Send(encodeSingleOpcode(clientpackets.OpcodeRequestRestart))
	assertFrameOpcode(t, h.second.Read(), serverpackets.OpcodeRestartResponse, "RestartResponse")
	h.awaitOffline(t, h.secondID)
	drainUntilQuiet(t, h.first)
	drainUntilQuiet(t, h.second)
	startInWorld(t, h.second)
	drainUntilQuiet(t, h.first)
	drainUntilQuiet(t, h.second)

	selectEnchantScroll(t, h.second, scroll)
	h.first.Send(encodeTradeDone(1))
	assertCancelPair(t, h.first, "TraderOne")
	assertSilent(t, h.second, "relogged partner after the old session's confirm")

	// The selection is still live: enchanting the +0 sword (inside the safe
	// range) succeeds without a new scroll prompt.
	h.second.Send(encodeRequestEnchantItem(weapon))
	drainUntilQuiet(t, h.second)
	sword := h.srv.PlayerInventory(t, h.secondID).ItemByObjectID(weapon)
	if sword == nil || sword.Snapshot().EnchantLevel != 1 {
		t.Fatalf("sword after the kept selection = %+v, want +1", sword)
	}
	if held := h.srv.PlayerInventory(t, h.secondID).ItemByObjectID(scroll); held != nil {
		t.Fatalf("scroll after the enchant = %+v, want consumed", held.Snapshot())
	}
}

// TestSelectedEnchantScrollCannotBeOffered pins the enchant clause of the
// item guard on AddTradeItem (Player.validateItemManipulation,
// Player.java:6213-6215): the scroll a trader selected before the window
// opened is refused with NOTHING_HAPPENED and the partner hears nothing.
// The same scroll, unselected, is offered as usual.
func TestSelectedEnchantScrollCannotBeOffered(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		selected bool
	}{{"selected", true}, {"not selected", false}} {
		t.Run(tc.name, func(t *testing.T) {
			h := bootTraders(t)
			scroll := h.srv.GiveItem(t, h.firstID, enchantScrollD, 1)
			h.enterAll(t)
			if tc.selected {
				selectEnchantScroll(t, h.first, scroll)
			}
			h.startTrade(t)

			h.first.Send(encodeAddTradeItem(0, scroll, 1))
			if !tc.selected {
				assertFrameOpcode(t, h.first.Read(), serverpackets.OpcodeTradeOwnAdd, "TradeOwnAdd")
				return
			}
			assertStaticSystemMessage(t, h.first.Read(), serverpackets.SystemMessageNothingHappened)
			assertSilent(t, h.second, "partner of a refused scroll offer")
			if held := h.srv.PlayerInventory(t, h.firstID).ItemByObjectID(scroll); held == nil || held.Snapshot().Count != 1 {
				t.Fatalf("scroll after the refused offer = %+v, want one still held", held)
			}
		})
	}
}
