package items

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference: RequestDestroyItem.java:103 ends with
// player.destroyItem(objectId, count, true), and Player.destroyItem
// (Player.java:1927-1949) then names what went: S1S_REMAINING_MANA_IS_NOW_0
// for a shadow item, S2_S1_DISAPPEARED (item name, item number = the
// requested count) when count > 1, otherwise S1_DISAPPEARED (item name).
// RequestDestroyItem.java:34-38 refuses first, before any item lookup, a
// player that isProcessingTransaction() or isOperating() with
// CANNOT_TRADE_DISCARD_DROP_ITEM_WHILE_IN_SHOPMODE.

const destroyShadowSwordID = 7884

// TestDestroyNamesWhatDisappeared destroys one item per case and requires
// exactly one SystemMessage, naming the item (and the destroyed count for
// more than one unit), and the stack left behind.
func TestDestroyNamesWhatDisappeared(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		templateID int32
		held       int
		destroy    int32
		wantID     int32
		wantParams []grantParam
	}{
		{
			name: "one sword", templateID: 30, held: 1, destroy: 1,
			wantID: serverpackets.SystemMessageS1Disappeared, wantParams: []grantParam{itemNameParam(30)},
		},
		{
			name: "three of a potion stack", templateID: 20, held: 5, destroy: 3,
			wantID: serverpackets.SystemMessageS2S1Disappeared, wantParams: []grantParam{itemNameParam(20), itemNumberParam(3)},
		},
		{
			name: "one of a potion stack", templateID: 20, held: 5, destroy: 1,
			wantID: serverpackets.SystemMessageS1Disappeared, wantParams: []grantParam{itemNameParam(20)},
		},
		{
			name: "whole potion stack", templateID: 20, held: 4, destroy: 4,
			wantID: serverpackets.SystemMessageS2S1Disappeared, wantParams: []grantParam{itemNameParam(20), itemNumberParam(4)},
		},
		{
			name: "adena", templateID: item.AdenaID, held: 100, destroy: 40,
			wantID: serverpackets.SystemMessageS2S1Disappeared, wantParams: []grantParam{itemNameParam(item.AdenaID), itemNumberParam(40)},
		},
		{
			name: "shadow sword", templateID: destroyShadowSwordID, held: 1, destroy: 1,
			wantID: serverpackets.SystemMessageRemainingManaIsNow0, wantParams: []grantParam{itemNameParam(destroyShadowSwordID)},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1))
			c := srv.Client
			objID := srv.SoleObjectID(t)
			target := srv.GiveItem(t, objID, tc.templateID, int32(tc.held))
			startInWorld(t, c)

			c.Send(encodeRequestDestroyItem(target, tc.destroy))
			var messages [][]byte
			for _, f := range collectUntilQuiet(t, c) {
				switch f[0] {
				case serverpackets.OpcodeSystemMessage:
					messages = append(messages, f)
				case serverpackets.OpcodeActionFailed:
					t.Fatal("destroy answered ActionFailed")
				}
			}
			if len(messages) != 1 {
				t.Fatalf("destroy sent %d SystemMessages, want 1", len(messages))
			}
			assertGrantMessage(t, messages[0], tc.wantID, tc.wantParams...)

			srv.InventoryUpdates.Tick()
			drainUntilQuiet(t, c)
			if left := tc.held - int(tc.destroy); left > 0 {
				assertItemCount(t, srv, objID, target, left)
			} else {
				srv.FlushItems(t)
				assertItemGone(t, srv, objID, target)
			}
		})
	}
}

// TestDestroyInStoreModeIsRefused: a player running a private store is
// answered CANNOT_TRADE_DISCARD_DROP_ITEM_WHILE_IN_SHOPMODE ahead of every
// other destroy check (even an unknown object id), and keeps the item.
func TestDestroyInStoreModeIsRefused(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	objID := srv.SoleObjectID(t)
	sword := srv.GiveItem(t, objID, 30, 1)
	startInWorld(t, c)
	srv.SetPlayerOperating(t, objID, true)

	c.Send(encodeRequestDestroyItem(sword, 1))
	assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageCannotTradeDiscardDropInShopMode)
	assertNoFrameFor(t, c, 300*time.Millisecond, "after the store-mode refusal")

	c.Send(encodeRequestDestroyItem(sword+1_000_000, 1))
	assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageCannotTradeDiscardDropInShopMode)
	assertNoFrameFor(t, c, 300*time.Millisecond, "after the unknown-item store-mode refusal")

	assertItemCount(t, srv, objID, sword, 1)
}
