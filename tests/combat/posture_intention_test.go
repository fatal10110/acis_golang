package combat

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

func TestAttackRequestWaitsForStandUp(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1))
	c, objID := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)
	hostile := srv.SpawnHostileNPCAt(t, location.Location{X: 40, Y: 20, Z: 30})
	drainUntilQuiet(t, c)
	targetHostile(t, c, hostile.ObjectID())
	sitPlayer(t, c)
	srv.SettlePosture(t, srv.SoleObjectID(t))
	c.Send(encodeRequestChangeWaitType(true))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeChangeWaitType, "stand")
	c.Send(encodeAttackRequest(hostile.ObjectID(), 10, 20, 30, false))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "queued attack")
	for _, frame := range testsupport.SyncBarrierFrames(t, c, func() {
		c.Send(encodeSingleOpcode(clientpackets.OpcodeRequestItemList))
	}, serverpackets.OpcodeItemList) {
		if frame[0] == serverpackets.OpcodeAttack || frame[0] == serverpackets.OpcodeMoveToPawn {
			t.Fatalf("attack started during stand-up with opcode %#x", frame[0])
		}
	}
	srv.Advance(t, 2500*time.Millisecond)
	assertAttackBy(t, c, objID)
}

func TestAttackQueuedDuringStandUpKeepsOriginalTarget(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1))
	c, objID := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)
	first := srv.SpawnHostileNPCAt(t, location.Location{X: 40, Y: 20, Z: 30})
	second := srv.SpawnHostileNPCAt(t, location.Location{X: 45, Y: 25, Z: 30})
	drainUntilQuiet(t, c)
	targetHostile(t, c, first.ObjectID())
	sitPlayer(t, c)
	srv.SettlePosture(t, srv.SoleObjectID(t))
	c.Send(encodeRequestChangeWaitType(true))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeChangeWaitType, "stand")
	c.Send(encodeAttackRequest(first.ObjectID(), 10, 20, 30, false))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "queued attack")
	targetHostile(t, c, second.ObjectID())
	srv.Advance(t, 2500*time.Millisecond)
	attack := assertAttackBy(t, c, objID)
	r := wireReader(attack[1:])
	r.ReadInt32()
	if got := r.ReadInt32(); got != first.ObjectID() {
		t.Fatalf("attack target = %d, want queued target %d", got, first.ObjectID())
	}
}

func TestShiftAttackQueuedDuringStandUpDoesNotWalk(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1))
	c, objID := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)
	hostile := srv.SpawnHostileNPCAt(t, location.Location{X: hostileX + 500, Y: hostileY, Z: hostileZ})
	drainUntilQuiet(t, c)
	targetHostile(t, c, hostile.ObjectID())
	sitPlayer(t, c)
	srv.SettlePosture(t, srv.SoleObjectID(t))
	c.Send(encodeRequestChangeWaitType(true))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeChangeWaitType, "stand")
	c.Send(encodeAttackRequest(hostile.ObjectID(), 10, 20, 30, true))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "queued shift attack")
	srv.Advance(t, 2500*time.Millisecond)
	assertHeldAttackIdle(t, srv, c, objID, playerOrigin, time.Second, true, "shift attack after stand-up")
}

func TestAttackRequestDuringSitDownIsRejectedAtSettlement(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	startInWorld(t, c)
	hostile := srv.SpawnHostileNPCAt(t, location.Location{X: 40, Y: 20, Z: 30})
	drainUntilQuiet(t, c)
	targetHostile(t, c, hostile.ObjectID())
	sitPlayer(t, c)
	c.Send(encodeAttackRequest(hostile.ObjectID(), 10, 20, 30, false))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "queued attack")
	srv.SettlePosture(t, srv.SoleObjectID(t))
	failed := false
	for _, frame := range testsupport.SyncBarrierFrames(t, c, func() {
		c.Send(encodeSingleOpcode(clientpackets.OpcodeRequestItemList))
	}, serverpackets.OpcodeItemList) {
		if frame[0] == serverpackets.OpcodeAttack || frame[0] == serverpackets.OpcodeMoveToPawn {
			t.Fatalf("seated player attacked with opcode %#x", frame[0])
		}
		failed = failed || frame[0] == serverpackets.OpcodeActionFailed
	}
	if !failed {
		t.Fatal("queued seated attack did not answer ActionFailed at settlement")
	}
	actor, ok := srv.State.Player(srv.SoleObjectID(t))
	if !ok || !actor.(interface{ Seated() bool }).Seated() {
		t.Fatal("player did not remain seated after queued attack")
	}
}

func TestFollowRequestWaitsForStandUp(t *testing.T) {
	t.Parallel()
	p := bootClickPair(t, 0)
	p.walkVictimAway(t, 300)
	selectPlayerTarget(t, p.c, p.victimID)
	sitPlayer(t, p.c)
	p.srv.SettlePosture(t, p.attackerID)
	p.c.Send(encodeRequestChangeWaitType(true))
	assertFrameOpcode(t, p.c.Read(), serverpackets.OpcodeChangeWaitType, "stand")
	p.c.Send(encodeAction(p.victimID, 10, 20, 30, false))
	assertFrameOpcode(t, p.c.Read(), serverpackets.OpcodeActionFailed, "queued follow")
	if x, _, _ := p.srv.PlayerPosition(t, p.attackerID); x != playerOrigin.X {
		t.Fatalf("follower moved during stand-up to x=%d", x)
	}
	p.srv.SettlePosture(t, p.attackerID)
	frames := readQuiet(p.c)
	if followMoveIndex(frames, p.attackerID, p.victimID) < 0 {
		t.Fatalf("frames after stand-up = %v, want queued follow MoveToPawn", opcodes(frames))
	}
}

func TestSitRequestWaitsForSwing(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 5, 0), gameservertest.WithWantChars(1))
	c, objID := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)
	hostile := srv.SpawnHostileNPCAt(t, location.Location{X: 40, Y: 20, Z: 30})
	drainUntilQuiet(t, c)
	targetHostile(t, c, hostile.ObjectID())
	c.Send(encodeAction(hostile.ObjectID(), 10, 20, 30, false))
	assertAttackBy(t, c, objID)
	c.Send(encodeRequestChangeWaitType(false))
	for _, frame := range testsupport.SyncBarrierFrames(t, c, func() {
		c.Send(encodeSingleOpcode(clientpackets.OpcodeRequestItemList))
	}, serverpackets.OpcodeItemList) {
		if frame[0] == serverpackets.OpcodeChangeWaitType || frame[0] == serverpackets.OpcodeActionFailed {
			t.Fatalf("sit answered during swing with opcode %#x", frame[0])
		}
	}
	mustReadOpcode := func() {
		for {
			frame := c.ReadWithTimeout(5 * time.Second)
			if frame == nil {
				t.Fatal("sit request never ran after swing")
			}
			if frame[0] == serverpackets.OpcodeChangeWaitType {
				return
			}
		}
	}
	mustReadOpcode()
}
