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
	srv.Advance(t, 2500*time.Millisecond)
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

func TestFollowRequestWaitsForStandUp(t *testing.T) {
	t.Parallel()
	p := bootClickPair(t, 0)
	p.walkVictimAway(t, 300)
	selectPlayerTarget(t, p.c, p.victimID)
	sitPlayer(t, p.c)
	p.srv.Advance(t, 2500*time.Millisecond)
	p.c.Send(encodeRequestChangeWaitType(true))
	assertFrameOpcode(t, p.c.Read(), serverpackets.OpcodeChangeWaitType, "stand")
	p.c.Send(encodeAction(p.victimID, 10, 20, 30, false))
	assertFrameOpcode(t, p.c.Read(), serverpackets.OpcodeActionFailed, "queued follow")
	if x, _, _ := p.srv.PlayerPosition(t, p.attackerID); x != playerOrigin.X {
		t.Fatalf("follower moved during stand-up to x=%d", x)
	}
	p.srv.Advance(t, 2500*time.Millisecond)
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
