package items

import (
	"context"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/admin"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// Access levels as accessLevels.xml ships them: 0 "User", 2 "Test GM"
// (allowTransaction="false", isGM="false") and 7 "Admin" (isGM="true").
const (
	userAccessLevel   = 0
	testGMAccessLevel = 2
	adminAccessLevel  = 7
)

// dropAccessLevels holds the drop-relevant rows of the shipped
// accessLevels.xml.
func dropAccessLevels(t *testing.T) *admin.Data {
	t.Helper()
	data, err := admin.NewData([]admin.AccessLevel{
		{Level: userAccessLevel, Name: "User", NameColor: "FFFFFF", TitleColor: "FFFF77", AllowTransaction: true, GiveDamage: true},
		{Level: testGMAccessLevel, Name: "Test GM", NameColor: "FFFFFF", TitleColor: "FFFF77", ChildLevel: 1, AllowFixedRes: true, AllowAltG: true},
		{Level: adminAccessLevel, Name: "Admin", NameColor: "CC6600", TitleColor: "CC6600", ChildLevel: 6, IsGM: true, AllowFixedRes: true, AllowTransaction: true, AllowAltG: true, GiveDamage: true},
	}, nil)
	if err != nil {
		t.Fatalf("admin.NewData: %v", err)
	}
	return data
}

// bootDropper boots one character holding 100 adena at the given access
// level, already in the world, and returns its server, id and adena stack.
func bootDropper(t *testing.T, accessLevel int, opts ...gameservertest.Option) (*gameservertest.Server, int32, int32) {
	t.Helper()
	opts = append([]gameservertest.Option{
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithAdmin(dropAccessLevels(t)),
	}, opts...)
	srv := gameservertest.Boot(t, opts...)
	objID := srv.SoleObjectID(t)
	if _, err := srv.DB.ExecContext(context.Background(), "UPDATE characters SET accesslevel = ? WHERE obj_Id = ?", accessLevel, objID); err != nil {
		t.Fatalf("set access level: %v", err)
	}
	adena := srv.GiveItem(t, objID, item.AdenaID, 100)
	startInWorld(t, srv.Client)
	return srv, objID, adena
}

// assertDropRefused sends a drop of objectID and requires exactly the
// refusal message, after which nothing has left the inventory or reached the
// ground.
func assertDropRefused(t *testing.T, srv *gameservertest.Server, c *testsupport.ScriptedClient, ownerID, objectID, count, x int32, messageID int) {
	t.Helper()
	before := mustFindItem(t, srv, ownerID, objectID).Count
	c.Send(encodeRequestDropItem(objectID, count, x, spawnY, spawnZ))
	assertStaticSystemMessage(t, c.Read(), messageID)
	if frame := c.ReadWithTimeout(300 * time.Millisecond); frame != nil {
		t.Fatalf("refused drop sent opcode %#x after its refusal, want nothing", frame[0])
	}
	if snaps := srv.GroundItems.Snapshots(nil); len(snaps) != 0 {
		t.Fatalf("ground items after a refused drop = %+v, want none", snaps)
	}
	srv.FlushItems(t)
	if got := mustFindItem(t, srv, ownerID, objectID).Count; got != before {
		t.Fatalf("item %d count after a refused drop = %d, want %d", objectID, got, before)
	}
}

// assertDropLands sends a drop of objectID in reach and requires its
// DropItem.
func assertDropLands(t *testing.T, c *testsupport.ScriptedClient, objectID, count int32) {
	t.Helper()
	c.Send(encodeRequestDropItem(objectID, count, spawnX, spawnY, spawnZ))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeDropItem, "DropItem")
}

// TestDropRefusedWithoutTransactionRight pins the access-level gate
// (RequestDropItem.java:61-65): a character whose access level forbids
// transactions is answered YOU_ARE_NOT_AUTHORIZED_TO_DO_THAT, in reach or
// not, and keeps its adena; a user-level character drops as usual.
func TestDropRefusedWithoutTransactionRight(t *testing.T) {
	t.Parallel()
	t.Run("test GM", func(t *testing.T) {
		t.Parallel()
		srv, objID, adena := bootDropper(t, testGMAccessLevel)
		assertDropRefused(t, srv, srv.Client, objID, adena, 40, spawnX, serverpackets.SystemMessageNotAuthorizedToDoThat)
		assertDropRefused(t, srv, srv.Client, objID, adena, 40, spawnX+1000, serverpackets.SystemMessageNotAuthorizedToDoThat)
	})
	t.Run("user", func(t *testing.T) {
		t.Parallel()
		srv, _, adena := bootDropper(t, userAccessLevel)
		assertDropLands(t, srv.Client, adena, 40)
	})
}

// TestDropRefusedWhileTradingOrInStore pins the shop-mode gate
// (RequestDropItem.java:67-71): a pending trade request, an open trade
// window or a store set up refuses the drop with
// CANNOT_TRADE_DISCARD_DROP_ITEM_WHILE_IN_SHOPMODE, ahead of the distance.
func TestDropRefusedWhileTradingOrInStore(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		setup func(t *testing.T, srv *gameservertest.Server, c, other *testsupport.ScriptedClient, objID, otherID int32)
	}{
		{"trade request pending", func(t *testing.T, _ *gameservertest.Server, c, other *testsupport.ScriptedClient, _, otherID int32) {
			c.Send(encodeTradeRequest(otherID))
			assertFrameOpcode(t, other.Read(), serverpackets.OpcodeSendTradeRequest, "SendTradeRequest")
			assertStaticSystemMessageText(t, c.Read())
		}},
		{"trade window open", func(t *testing.T, _ *gameservertest.Server, c, other *testsupport.ScriptedClient, objID, otherID int32) {
			openTrade(t, c, other, objID, otherID)
		}},
		{"store set up", func(t *testing.T, srv *gameservertest.Server, _, _ *testsupport.ScriptedClient, objID, _ int32) {
			srv.SetPlayerOperating(t, objID, true)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv, objID, adena := bootDropper(t, userAccessLevel)
			c := srv.Client
			second := srv.SeedCharacterFor(t, "player2", "Second", 1, 0)
			other := srv.DialClient(t, "player2", 1)
			startInWorld(t, other)
			drainUntilQuiet(t, other)
			drainUntilQuiet(t, c)

			tc.setup(t, srv, c, other, objID, second.ID)
			assertDropRefused(t, srv, c, objID, adena, 40, spawnX, serverpackets.SystemMessageCannotTradeDiscardDropInShopMode)
			assertDropRefused(t, srv, c, objID, adena, 40, spawnX+1000, serverpackets.SystemMessageCannotTradeDiscardDropInShopMode)
		})
	}
}

// TestDropRefusedWhileFishing pins the fishing gate
// (RequestDropItem.java:73-77): CANNOT_DO_WHILE_FISHING_2, ahead of the
// distance; the same drop lands once the line is in.
func TestDropRefusedWhileFishing(t *testing.T) {
	t.Parallel()
	srv, objID, adena := bootDropper(t, userAccessLevel)
	srv.SetPlayerFishing(t, objID, true)
	assertDropRefused(t, srv, srv.Client, objID, adena, 40, spawnX, serverpackets.SystemMessageCannotDoWhileFishing2)
	assertDropRefused(t, srv, srv.Client, objID, adena, 40, spawnX+1000, serverpackets.SystemMessageCannotDoWhileFishing2)

	srv.SetPlayerFishing(t, objID, false)
	assertDropLands(t, srv.Client, adena, 40)
}

// TestDropRefusesSelectedEnchantScroll pins the enchant clause of the item
// guard (Player.validateItemManipulation, Player.java:6213-6215): the scroll
// the player has selected cannot be dropped (CANNOT_DISCARD_THIS_ITEM) and
// stays selected. Once the selection is cancelled the scroll drops.
func TestDropRefusesSelectedEnchantScroll(t *testing.T) {
	t.Parallel()
	srv, objID, _, scroll := bootEnchanter(t, func() float64 { return 0 }, 0, false, nil)
	c := srv.Client
	openEnchantSelection(t, c, scroll, 955)

	assertDropRefused(t, srv, c, objID, scroll, 1, spawnX, serverpackets.SystemMessageCannotDiscardThisItem)

	// The selection survived the refusal: a move cancels it with the
	// cancel pair, after which the scroll is an ordinary item again.
	c.Send(encodeMoveBackwardToLocation(spawnX+5, spawnY, spawnZ, spawnX, spawnY, spawnZ))
	sawCancel := false
	for _, frame := range collectUntilQuiet(t, c) {
		if frame[0] == serverpackets.OpcodeSystemMessage && systemMessageID(t, frame) == serverpackets.SystemMessageEnchantScrollCancelled {
			sawCancel = true
		}
	}
	if !sawCancel {
		t.Fatal("moving after the refused drop did not cancel the scroll selection")
	}
	c.Send(encodeRequestDropItem(scroll, 1, spawnX+5, spawnY, spawnZ))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeDropItem, "DropItem of the released scroll")
}

// TestDropDisabledByConfig pins AllowDiscardItem = False
// (RequestDropItem.java:40): every drop by a non-GM is refused with
// CANNOT_DISCARD_THIS_ITEM, ahead of the distance; a GM still drops.
func TestDropDisabledByConfig(t *testing.T) {
	t.Parallel()
	t.Run("player", func(t *testing.T) {
		t.Parallel()
		srv, objID, adena := bootDropper(t, userAccessLevel, gameservertest.WithDiscardItemDisabled())
		assertDropRefused(t, srv, srv.Client, objID, adena, 40, spawnX, serverpackets.SystemMessageCannotDiscardThisItem)
		assertDropRefused(t, srv, srv.Client, objID, adena, 40, spawnX+1000, serverpackets.SystemMessageCannotDiscardThisItem)
	})
	t.Run("GM", func(t *testing.T) {
		t.Parallel()
		srv, _, adena := bootDropper(t, adminAccessLevel, gameservertest.WithDiscardItemDisabled())
		drainUntilQuiet(t, srv.Client)
		assertDropLands(t, srv.Client, adena, 40)
	})
}
