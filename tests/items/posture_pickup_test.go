package items

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

func TestPickupWaitsForStandUp(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 1, 0), gameservertest.WithWantChars(1))
	c, objID := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)
	srv.SeedGroundItem(t, 0, item.AdenaID, 100, spawnX, spawnY, spawnZ)
	drainUntilQuiet(t, c)
	groundID := soleGroundObjectID(t, srv)
	sitAndSettle(t, srv)
	changePosture(t, c, true)
	c.Send(encodeAction(groundID, spawnX, spawnY, spawnZ, false))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "queued pickup")
	barrier(t, c)
	if _, ok := srv.State.Object(groundID); !ok {
		t.Fatal("ground item collected during stand-up")
	}
	srv.Advance(t, 2500*time.Millisecond)
	for {
		frame := c.ReadWithTimeout(5 * time.Second)
		if frame == nil {
			t.Fatal("queued pickup never completed")
		}
		if frame[0] == serverpackets.OpcodeGetItem {
			break
		}
	}
	srv.FlushItems(t)
	if got := carriedCount(t, srv, objID, item.AdenaID); got != 100 {
		t.Fatalf("persisted adena = %d, want 100", got)
	}
}

func TestPickupDuringSitDownIsRejectedAtSettlement(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 1, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	startInWorld(t, c)
	srv.SeedGroundItem(t, 0, item.AdenaID, 100, spawnX, spawnY, spawnZ)
	drainUntilQuiet(t, c)
	groundID := soleGroundObjectID(t, srv)
	changePosture(t, c, false)
	c.Send(encodeAction(groundID, spawnX, spawnY, spawnZ, false))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "queued pickup")
	srv.Advance(t, 2500*time.Millisecond)
	failed := false
	for _, frame := range testsupport.SyncBarrierFrames(t, c, func() { c.Send(encodeRequestItemList()) }, serverpackets.OpcodeItemList) {
		if frame[0] == serverpackets.OpcodeGetItem {
			t.Fatal("seated player received GetItem")
		}
		failed = failed || frame[0] == serverpackets.OpcodeActionFailed
	}
	if !failed {
		t.Fatal("queued seated pickup did not answer ActionFailed at settlement")
	}
	if _, ok := srv.State.Object(groundID); !ok {
		t.Fatal("seated player collected queued pickup")
	}
}

func TestPickupReplacesQueuedEquipDuringStandUp(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 1, 0), gameservertest.WithWantChars(1))
	c, objID := srv.Client, srv.SoleObjectID(t)
	sword := srv.GiveItem(t, objID, swordID, 1)
	startInWorld(t, c)
	srv.SeedGroundItem(t, 0, item.AdenaID, 100, spawnX, spawnY, spawnZ)
	drainUntilQuiet(t, c)
	groundID := soleGroundObjectID(t, srv)
	sitAndSettle(t, srv)
	changePosture(t, c, true)
	c.Send(encodeUseItem(sword, false))
	barrier(t, c)
	c.Send(encodeAction(groundID, spawnX, spawnY, spawnZ, false))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "queued pickup")
	srv.Advance(t, 2500*time.Millisecond)
	for {
		frame := c.ReadWithTimeout(5 * time.Second)
		if frame == nil {
			t.Fatal("queued pickup never completed")
		}
		if frame[0] == serverpackets.OpcodeGetItem {
			break
		}
	}
	assertRightHandEmpty(t, srv, objID, "after pickup replaced queued equip")
	srv.FlushItems(t)
	if got := carriedCount(t, srv, objID, item.AdenaID); got != 100 {
		t.Fatalf("persisted adena = %d, want 100", got)
	}
}
