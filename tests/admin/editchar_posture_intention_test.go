package admin

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

func encodeChangeWaitType(stand bool) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestChangeWaitType)
	w.WriteInt32(wire.BoolInt32(stand))
	return w.Bytes()
}

// charInfoNextAI returns the next intention //debug shows for Player. The
// current one is not pinned here: during a sit-down or stand-up the
// reference names SIT or STAND (#3839).
func charInfoNextAI(t *testing.T, srv *gameservertest.Server) string {
	t.Helper()
	line := charInfoAI(t, srv)
	_, next, ok := strings.Cut(line, " <> ")
	if !ok {
		t.Fatalf("intention line %q has no next intention", line)
	}
	return next
}

// standingUp has user sit down, lets the sit-down run out, and starts the
// stand-up, leaving user in the stand-up transition with nothing queued.
func standingUp(t *testing.T, srv *gameservertest.Server, user *testsupport.ScriptedClient, userID int32) {
	t.Helper()
	user.Send(encodeChangeWaitType(false))
	if !slices.Contains(opcodes(settle(t, user)), serverpackets.OpcodeChangeWaitType) {
		t.Fatal("sit request did not sit Player down")
	}
	srv.SettlePosture(t, userID)
	settle(t, user)
	user.Send(encodeChangeWaitType(true))
	if !slices.Contains(opcodes(settle(t, user)), serverpackets.OpcodeChangeWaitType) {
		t.Fatal("stand request did not start the stand-up")
	}
	if got := charInfoNextAI(t, srv); got != "IDLE" {
		t.Fatalf("next intention mid stand-up = %q, want IDLE", got)
	}
}

// TestAdminCharacterPagePostureQueuedIntention pins %nextai% (AdminEditChar
// gatherPlayerInfo, getNextIntention().getType()) naming a request clicked
// during a stand-up, which the reference stores as the next intention
// (PlayableAI tryToAttack, tryToPickUp, tryToFollow, tryToMoveTo, tryToSit:
// isStandingNow holds it).
func TestAdminCharacterPagePostureQueuedIntention(t *testing.T) {
	t.Parallel()

	expectQueued := func(t *testing.T, frames [][]byte, srv *gameservertest.Server, want string) {
		t.Helper()
		if !slices.Contains(opcodes(frames), serverpackets.OpcodeActionFailed) {
			t.Fatalf("mid stand-up request frames = %x, want it queued with ActionFailed", testsupport.FrameOpcodes(frames))
		}
		if got := charInfoNextAI(t, srv); got != want {
			t.Fatalf("next intention after the mid stand-up request = %q, want %q", got, want)
		}
	}

	t.Run("attack", func(t *testing.T) {
		t.Parallel()
		srv, user, userID, first, _ := bootNextAttackAdmin(t)
		standingUp(t, srv, user, userID)
		expectQueued(t, clickAttack(t, user, first), srv, "ATTACK")
		srv.SettlePosture(t, userID)
		if !slices.Contains(opcodes(settle(t, user)), serverpackets.OpcodeAttack) {
			t.Fatal("queued attack did not swing once the stand-up ended")
		}
	})

	t.Run("pickup", func(t *testing.T) {
		t.Parallel()
		srv, user, userID, _, _ := bootNextAttackAdmin(t)
		srv.SeedGroundItem(t, 0, item.AdenaID, 10, spawnX, spawnY, spawnZ)
		settle(t, user)
		snaps := srv.GroundItems.Snapshots(nil)
		if len(snaps) != 1 {
			t.Fatalf("ground items = %d, want 1", len(snaps))
		}
		standingUp(t, srv, user, userID)
		user.Send(encodeAction(snaps[0].ObjectID))
		expectQueued(t, settle(t, user), srv, "PICK_UP")
		srv.SettlePosture(t, userID)
		if !slices.Contains(opcodes(settle(t, user)), serverpackets.OpcodeGetItem) {
			t.Fatal("queued pickup did not pick the adena up once the stand-up ended")
		}
	})

	t.Run("follow", func(t *testing.T) {
		t.Parallel()
		srv, user, userID, _, _ := bootNextAttackAdmin(t)
		var gm int32
		if err := srv.DB.QueryRowContext(context.Background(), "SELECT obj_Id FROM characters WHERE char_name = 'Admin'").Scan(&gm); err != nil {
			t.Fatalf("read Admin's object id: %v", err)
		}
		standingUp(t, srv, user, userID)
		expectQueued(t, clickAttack(t, user, gm), srv, "FOLLOW")
	})

	t.Run("move", func(t *testing.T) {
		t.Parallel()
		srv, user, userID, _, _ := bootNextAttackAdmin(t)
		standingUp(t, srv, user, userID)
		user.Send(encodeMove(spawnX+200, spawnY, spawnZ))
		expectQueued(t, settle(t, user), srv, "MOVE_TO")
		srv.SettlePosture(t, userID)
		if !slices.Contains(opcodes(settle(t, user)), serverpackets.OpcodeMoveToLocation) {
			t.Fatal("queued walk did not start once the stand-up ended")
		}
	})

	t.Run("sit", func(t *testing.T) {
		t.Parallel()
		srv, user, userID, _, _ := bootNextAttackAdmin(t)
		standingUp(t, srv, user, userID)
		user.Send(encodeChangeWaitType(false))
		settle(t, user)
		if got := charInfoNextAI(t, srv); got != "SIT" {
			t.Fatalf("next intention after a mid stand-up sit = %q, want SIT", got)
		}
	})
}
