package trade

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
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
