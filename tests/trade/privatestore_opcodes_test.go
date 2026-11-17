package trade

import (
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/privatestore"
)

// TestBuyStoreManageAndQuitRequests drives the two buy-store requests that
// carry no body: the manage request opens the buy list window and leaves the
// player managing it, and the quit request ends the managing at once with a
// UserInfo to the owner and the world's view of it for the others.
func TestBuyStoreManageAndQuitRequests(t *testing.T) {
	t.Parallel()
	h := bootTraders(t)
	h.enterAll(t)

	h.first.Send(encodeSingleOpcode(clientpackets.OpcodeRequestPrivateStoreManageBuy))
	h.srv.Settle(t)
	owner := drainFrames(t, h.first)
	if len(framesWith(owner, serverpackets.OpcodePrivateStoreManageListBuy)) != 1 {
		t.Fatalf("manage request answered opcodes %x, want one PrivateStoreManageListBuy", opcodeList(owner))
	}
	if got := h.srv.PlayerOperateType(t, h.firstID); got != privatestore.OperateBuyManage {
		t.Fatalf("operate type while managing = %d, want buy manage", got)
	}
	drainFrames(t, h.second)

	h.first.Send(encodeSingleOpcode(clientpackets.OpcodeRequestPrivateStoreQuitBuy))
	h.srv.Settle(t)
	if got := h.srv.PlayerOperateType(t, h.firstID); got != privatestore.OperateNone {
		t.Fatalf("operate type after quitting = %d, want none", got)
	}
	assertOrder(t, drainFrames(t, h.first), "owner after quitting", serverpackets.OpcodeUserInfo)
	charInfoFor(t, drainFrames(t, h.second), h.firstID)
}

// TestSellStoreTitleRequest sets the sell store's title: every player has a
// sell list, so the owner is answered with PrivateStoreMsgSell and the title
// even before any store was opened; one past the 29-character limit is
// dropped without an answer.
func TestSellStoreTitleRequest(t *testing.T) {
	t.Parallel()
	h := bootTraders(t)
	h.enterAll(t)

	for _, title := range []string{"Potions cheap", strings.Repeat("x", 29), ""} {
		h.first.Send(encodeStoreText(clientpackets.OpcodeSetPrivateStoreMsgSell, title))
		frame := h.first.Read()
		assertFrameOpcode(t, frame, serverpackets.OpcodePrivateStoreMsgSell, "title answer")
		r := wire.NewReader(frame[1:])
		if owner, text := r.ReadInt32(), r.ReadString(); owner != h.firstID || text != title {
			t.Fatalf("PrivateStoreMsgSell = %d %q, want %d %q", owner, text, h.firstID, title)
		}
	}
	h.first.Send(encodeStoreText(clientpackets.OpcodeSetPrivateStoreMsgSell, strings.Repeat("x", 30)))
	assertSilent(t, h.first, "title over the limit")
}

// TestRecipeShopManagePrevRequest asks to preview a workshop: with no player
// targeted nothing is answered; with another player targeted the window shows
// that player's workshop, here empty, against the asker's adena.
func TestRecipeShopManagePrevRequest(t *testing.T) {
	t.Parallel()
	h := bootTraders(t)
	h.enterAll(t)

	h.first.Send(encodeSingleOpcode(clientpackets.OpcodeRequestRecipeShopManagePrev))
	assertSilent(t, h.first, "preview without a target")

	h.first.Send(encodeAction(h.secondID))
	h.srv.Settle(t)
	drainFrames(t, h.first)
	h.first.Send(encodeSingleOpcode(clientpackets.OpcodeRequestRecipeShopManagePrev))
	frame := h.first.Read()
	assertFrameOpcode(t, frame, serverpackets.OpcodeRecipeShopSellList, "preview of an empty workshop")
	r := wire.NewReader(frame[1:])
	if owner := r.ReadInt32(); owner != h.secondID {
		t.Fatalf("workshop owner = %d, want %d", owner, h.secondID)
	}
	r.ReadInt32() // current MP
	r.ReadInt32() // maximum MP
	r.ReadInt32() // the asker's adena
	if rows := r.ReadInt32(); rows != 0 {
		t.Fatalf("workshop rows = %d, want 0", rows)
	}
}
