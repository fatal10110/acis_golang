package admin

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// actionPrivateStoreSell is the RequestActionUse id of the sell store
// set-up.
const actionPrivateStoreSell = 10

// storeItemID is a plain stackable item the sell store lists.
const storeItemID = 20

func encodeSetPrivateStoreListSell(objectID, count, price int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeSetPrivateStoreListSell)
	w.WriteInt32(0)
	w.WriteInt32(1)
	w.WriteInt32(objectID)
	w.WriteInt32(count)
	w.WriteInt32(price)
	return w.Bytes()
}

// TestAdminCharacterPagePostureCurrentIntention pins %ai% (AdminEditChar
// gatherPlayerInfo, getCurrentIntention().getType()) during a sit or stand
// request: PlayerAI doSitIntention / doStandIntention make SIT or STAND the
// current intention until onEvtSatDown / onEvtStoodUp settle it, a request
// clicked meanwhile queued behind it. A store set-up sits the player down
// without that intention.
func TestAdminCharacterPagePostureCurrentIntention(t *testing.T) {
	t.Parallel()

	expectAI := func(t *testing.T, srv *gameservertest.Server, when, want string) {
		t.Helper()
		if got := charInfoAI(t, srv); got != want {
			t.Fatalf("intentions %s = %q, want %q", when, got, want)
		}
	}

	t.Run("sit", func(t *testing.T) {
		t.Parallel()
		srv, user, userID, _, _ := bootNextAttackAdmin(t)
		user.Send(encodeChangeWaitType(false))
		if !slices.Contains(opcodes(settle(t, user)), serverpackets.OpcodeChangeWaitType) {
			t.Fatal("sit request did not sit Player down")
		}
		expectAI(t, srv, "during the sit-down", "SIT <> IDLE")
		srv.SettlePosture(t, userID)
		settle(t, user)
		expectAI(t, srv, "once seated", "IDLE <> IDLE")
	})

	t.Run("stand", func(t *testing.T) {
		t.Parallel()
		srv, user, userID, _, _ := bootNextAttackAdmin(t)
		standingUp(t, srv, user, userID)
		expectAI(t, srv, "during the stand-up", "STAND <> IDLE")
		srv.SettlePosture(t, userID)
		settle(t, user)
		expectAI(t, srv, "once standing", "IDLE <> IDLE")
	})

	t.Run("stand with an attack queued", func(t *testing.T) {
		t.Parallel()
		srv, user, userID, first, _ := bootNextAttackAdmin(t)
		standingUp(t, srv, user, userID)
		clickAttack(t, user, first)
		expectAI(t, srv, "with an attack queued mid stand-up", "STAND <> ATTACK")
	})

	t.Run("store sit", func(t *testing.T) {
		t.Parallel()
		srv, _ := bootAdmin(t, adminLevel, gameservertest.WithHTMLPages(shippedAdminPages(t, "charinfo.htm")))
		if !srv.DrivesClock() {
			t.Skip("holding a sit-down open needs the driven clock")
		}
		enterWorld(t, srv.Client)
		ch := srv.SeedCharacterFor(t, "player2", "Player", 1, 0)
		potions := srv.GiveItem(t, ch.ID, storeItemID, 5)
		user := srv.DialClient(t, "player2", 1)
		enterWorld(t, user)
		drain(t, user)
		drain(t, srv.Client)

		user.Send(encodeActionUse(actionPrivateStoreSell))
		if !slices.Contains(opcodes(settle(t, user)), serverpackets.OpcodePrivateStoreManageListSell) {
			t.Fatal("sell command did not open the manage window")
		}
		user.Send(encodeSetPrivateStoreListSell(potions, 5, 10))
		frames := settle(t, user)
		if !slices.Contains(opcodes(frames), serverpackets.OpcodeChangeWaitType) {
			t.Fatalf("store set-up frames = %x, want it to sit Player down", opcodes(frames))
		}
		expectAI(t, srv, "during the store's sit-down", "IDLE <> IDLE")
	})
}
