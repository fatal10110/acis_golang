package character

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

func TestStandRequestWaitsForSitDown(t *testing.T) {
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 1, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	enterWorld(t, c)
	drainQuiet(t, c)

	c.Send(encodeRequestChangeWaitType(false))
	mustReadOpcode(t, c, serverpackets.OpcodeChangeWaitType, "sit")
	c.Send(encodeRequestChangeWaitType(true))
	for _, frame := range testsupport.SyncBarrierFrames(t, c, func() {
		c.Send(encodeSingleOpcode(clientpackets.OpcodeRequestItemList))
	}, serverpackets.OpcodeItemList) {
		if frame[0] == serverpackets.OpcodeChangeWaitType || frame[0] == serverpackets.OpcodeActionFailed {
			t.Fatalf("stand answered during sit-down with opcode %#x", frame[0])
		}
	}
	srv.Advance(t, 2500*time.Millisecond)
	mustReadOpcode(t, c, serverpackets.OpcodeChangeWaitType, "queued stand")
}

func TestMoveRequestWaitsForStandUp(t *testing.T) {
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 1, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	enterWorld(t, c)
	drainQuiet(t, c)
	c.Send(encodeRequestChangeWaitType(false))
	mustReadOpcode(t, c, serverpackets.OpcodeChangeWaitType, "sit")
	srv.Advance(t, 2500*time.Millisecond)
	c.Send(encodeRequestChangeWaitType(true))
	mustReadOpcode(t, c, serverpackets.OpcodeChangeWaitType, "stand")

	target := location.Location{X: 80, Y: 70, Z: 30}
	c.Send(encodeMoveBackwardToLocation(target, spawnOrigin, 1))
	mustReadOpcode(t, c, serverpackets.OpcodeActionFailed, "queued move")
	for _, frame := range testsupport.SyncBarrierFrames(t, c, func() {
		c.Send(encodeSingleOpcode(clientpackets.OpcodeRequestItemList))
	}, serverpackets.OpcodeItemList) {
		if frame[0] == serverpackets.OpcodeMoveToLocation {
			t.Fatal("walk started during stand-up")
		}
	}
	srv.Advance(t, 2500*time.Millisecond)
	mustReadOpcode(t, c, serverpackets.OpcodeMoveToLocation, "queued move after stand-up")
}

func TestChairReleasedAfterStandUp(t *testing.T) {
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 1, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	enterWorld(t, c)
	drainQuiet(t, c)
	chair := spawnChair(t, srv, c, nil)
	sitOnChair(t, c, nil, chair)
	srv.Advance(t, 2500*time.Millisecond)
	c.Send(encodeRequestChangeWaitType(true))
	mustReadOpcode(t, c, serverpackets.OpcodeChangeWaitType, "stand from chair")
	if !chair.Busy() {
		t.Fatal("chair freed before stand-up settled")
	}
	srv.Advance(t, 2499*time.Millisecond)
	if !chair.Busy() {
		t.Fatal("chair freed during stand-up")
	}
	srv.Advance(t, time.Millisecond)
	if chair.Busy() {
		t.Fatal("chair still busy after stand-up settled")
	}
}

func TestDamageDuringSitDownDoesNotStandPlayer(t *testing.T) {
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 1, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	enterWorld(t, c)
	drainQuiet(t, c)
	objID := srv.SoleObjectID(t)
	actor, ok := srv.State.Player(objID)
	if !ok {
		t.Fatal("player missing")
	}
	posture := actor.(interface {
		SittingNow() bool
		StandingNow() bool
		Seated() bool
	})
	hit := actor.(interface {
		ReduceHP(float64, attackable.Combatant, modelskill.Definition)
	})

	c.Send(encodeRequestChangeWaitType(false))
	mustReadOpcode(t, c, serverpackets.OpcodeChangeWaitType, "sit")
	hit.ReduceHP(1, nil, modelskill.Definition{})
	if !posture.SittingNow() || posture.StandingNow() {
		t.Fatal("hit stood the player during sit-down")
	}
	for _, frame := range testsupport.SyncBarrierFrames(t, c, func() {
		c.Send(encodeSingleOpcode(clientpackets.OpcodeRequestItemList))
	}, serverpackets.OpcodeItemList) {
		if frame[0] == serverpackets.OpcodeChangeWaitType {
			t.Fatal("hit broadcast a stand during sit-down")
		}
	}
	srv.Advance(t, 2500*time.Millisecond)
	if !posture.Seated() {
		t.Fatal("player did not finish sitting down")
	}
	hit.ReduceHP(1, nil, modelskill.Definition{})
	if !posture.StandingNow() {
		t.Fatal("hit did not stand the seated player")
	}
}

func TestChairInteractWaitsForStandUp(t *testing.T) {
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 1, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	enterWorld(t, c)
	drainQuiet(t, c)
	chair := spawnChair(t, srv, c, nil)
	c.Send(encodeRequestChangeWaitType(false))
	mustReadOpcode(t, c, serverpackets.OpcodeChangeWaitType, "sit")
	srv.Advance(t, 2500*time.Millisecond)
	c.Send(encodeAction(chair.ObjectID(), 10, 20, 30, false))
	mustReadOpcode(t, c, serverpackets.OpcodeMyTargetSelected, "select chair")
	c.Send(encodeRequestChangeWaitType(true))
	mustReadOpcode(t, c, serverpackets.OpcodeChangeWaitType, "stand")
	c.Send(encodeAction(chair.ObjectID(), 10, 20, 30, false))
	mustReadOpcode(t, c, serverpackets.OpcodeActionFailed, "queued chair interact")
	if chair.Busy() {
		t.Fatal("chair claimed during stand-up")
	}
	srv.Advance(t, 2500*time.Millisecond)
	mustReadOpcode(t, c, serverpackets.OpcodeChangeWaitType, "chair sit after stand-up")
	if !chair.Busy() {
		t.Fatal("chair not claimed after queued interact")
	}
	mustReadOpcode(t, c, serverpackets.OpcodeChairSit, "chair sit")
}

func TestStandRequestDuringFakeDeathGetUpAnswersAtSettlement(t *testing.T) {
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 1, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	enterWorld(t, c)
	drainQuiet(t, c)
	actor, ok := srv.State.Player(srv.SoleObjectID(t))
	if !ok {
		t.Fatal("player missing")
	}
	fake := actor.(interface {
		StartFakeDeath() bool
		StopFakeDeath() bool
		StandingNow() bool
	})
	fake.StartFakeDeath()
	mustReadOpcode(t, c, serverpackets.OpcodeChangeWaitType, "fake death")
	srv.Advance(t, 3*time.Second)
	fake.StopFakeDeath()
	mustReadOpcode(t, c, serverpackets.OpcodeChangeWaitType, "fake death get-up")
	if !fake.StandingNow() {
		t.Fatal("get-up transition did not start")
	}
	c.Send(encodeRequestChangeWaitType(true))
	for _, frame := range testsupport.SyncBarrierFrames(t, c, func() {
		c.Send(encodeSingleOpcode(clientpackets.OpcodeRequestItemList))
	}, serverpackets.OpcodeItemList) {
		if frame[0] == serverpackets.OpcodeActionFailed {
			t.Fatal("stand request answered during get-up")
		}
	}
	srv.Advance(t, 2500*time.Millisecond)
	mustReadOpcode(t, c, serverpackets.OpcodeActionFailed, "queued stand rejection")
}
