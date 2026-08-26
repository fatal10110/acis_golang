package combat

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestWalkAwayStopsAutoAttack pins the stance-off half of the attack flow: a
// client-initiated walk cancels the active attack, so no further swings land
// after the move.
func TestWalkAwayStopsAutoAttack(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c := srv.Client
	startInWorld(t, c)
	hostile := srv.SpawnHostileNPCAt(t, location.Location{X: hostileX - 20, Y: hostileY, Z: hostileZ})
	drainUntilQuiet(t, c)

	targetHostile(t, c, hostile.ObjectID())
	c.Send(encodeAction(hostile.ObjectID(), int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))
	assertAutoAttackStart(t, c, srv.SoleObjectID(t))
	waitFor(t, "opening swing", func() bool { return hostile.CurrentHP() < hostile.MaxHP() })

	c.Send(encodeMoveBackwardToLocation(-2000, 2000, 30))
	drainUntilQuiet(t, c)

	afterMove := hostile.CurrentHP()
	time.Sleep(1500 * time.Millisecond)
	if got := hostile.CurrentHP(); got != afterMove {
		t.Fatalf("hostile HP = %d after walking away, want frozen at %d (attack not stopped)", got, afterMove)
	}
}

// TestAttackWalksIntoRangeThenLandsSwing pins the out-of-range attack flow:
// MoveToPawn starts the approach and the first swing lands only after real
// arrival — the world-grid arrival path, not a premature hit.
func TestAttackWalksIntoRangeThenLandsSwing(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c := srv.Client
	startInWorld(t, c)
	hostile := srv.SpawnHostileNPCAt(t, location.Location{X: hostileX + 150, Y: hostileY, Z: hostileZ})
	drainUntilQuiet(t, c)

	targetHostile(t, c, hostile.ObjectID())
	c.Send(encodeAction(hostile.ObjectID(), int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))
	assertFrameOpcode(t, mustRead(t, c, "MoveToPawn"), serverpackets.OpcodeMoveToPawn, "approach")

	waitFor(t, "post-arrival swing", func() bool { return hostile.CurrentHP() < hostile.MaxHP() })
	drainUntilQuiet(t, c)
}

// TestTargetCancelStopsSwingLoop pins that cancelling the target ends the
// swing loop: the cancel answers ActionFailed and no further swings land.
func TestTargetCancelStopsSwingLoop(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c := srv.Client
	startInWorld(t, c)
	hostile := srv.SpawnHostileNPCAt(t, location.Location{X: hostileX - 20, Y: hostileY, Z: hostileZ})
	drainUntilQuiet(t, c)

	targetHostile(t, c, hostile.ObjectID())
	c.Send(encodeAction(hostile.ObjectID(), int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))
	assertAutoAttackStart(t, c, srv.SoleObjectID(t))
	waitFor(t, "opening swing", func() bool { return hostile.CurrentHP() < hostile.MaxHP() })

	c.Send(encodeRequestTargetCancel(1))
	// The swing loop may broadcast one more in-flight Attack before the
	// cancel lands; skip such trailing combat frames to reach the ack.
	for {
		frame := mustRead(t, c, "cancel ack")
		if frame[0] == serverpackets.OpcodeActionFailed {
			break
		}
		if frame[0] != serverpackets.OpcodeAttack && frame[0] != serverpackets.OpcodeStatusUpdate {
			t.Fatalf("cancel ack opcode = %#x, want ActionFailed", frame[0])
		}
	}
	drainUntilQuiet(t, c)
	afterCancel := hostile.CurrentHP()
	time.Sleep(1200 * time.Millisecond)
	if got := hostile.CurrentHP(); got != afterCancel {
		t.Fatalf("hostile HP = %d after cancel, want frozen at %d", got, afterCancel)
	}
}

// silentStanceEffects drops the combat-stance expiry broadcast: every
// scenario here finishes well inside the 15s stance window, and the refusal
// assertions only need the tracker to keep reporting the stance.
type silentStanceEffects struct{}

func (silentStanceEffects) AutoAttackStop(task.AttackStanceActor) {}

// TestAttackStanceBlocksRestartAndLogout pins that a character whose combat
// stance is still active after disengaging refuses restart with
// CANNOT_RESTART_WHILE_FIGHTING + RestartResponse(false) and logout with
// CANNOT_LOGOUT_WHILE_FIGHTING + ActionFailed.
func TestAttackStanceBlocksRestartAndLogout(t *testing.T) {
	stance, err := task.NewAttackStance(silentStanceEffects{}, time.Now)
	if err != nil {
		t.Fatalf("build attack stance tracker: %v", err)
	}
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithAttackStance(stance),
	)
	c := srv.Client
	startInWorld(t, c)
	hostile := srv.SpawnHostileNPCAt(t, location.Location{X: hostileX - 20, Y: hostileY, Z: hostileZ})
	drainUntilQuiet(t, c)

	targetHostile(t, c, hostile.ObjectID())
	c.Send(encodeAction(hostile.ObjectID(), int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))
	assertAutoAttackStart(t, c, srv.SoleObjectID(t))
	waitFor(t, "opening swing", func() bool { return hostile.CurrentHP() < hostile.MaxHP() })

	// Disengage: the walk stops the swing loop but the combat stance stays
	// registered until its inactivity period expires.
	c.Send(encodeMoveBackwardToLocation(-2000, 2000, 30))
	drainUntilQuiet(t, c)

	c.Send(encodeSingleOpcode(clientpackets.OpcodeRequestRestart))
	assertSystemMessageNumber(t, mustRead(t, c, "restart refusal"), serverpackets.SystemMessageCannotRestartWhileFighting)
	reply := mustRead(t, c, "RestartResponse")
	assertFrameOpcode(t, reply, serverpackets.OpcodeRestartResponse, "RestartResponse")
	if ok := wire.NewReader(reply[1:]).ReadInt32(); ok != 0 {
		t.Fatalf("RestartResponse result = %d, want 0", ok)
	}
	if _, ok := srv.State.Player(srv.SoleObjectID(t)); !ok {
		t.Fatal("player left the world despite refused restart")
	}

	c.Send(encodeLogout())
	assertSystemMessageNumber(t, mustRead(t, c, "logout refusal"), serverpackets.SystemMessageCannotLogoutWhileFighting)
	assertFrameOpcode(t, mustRead(t, c, "ActionFailed"), serverpackets.OpcodeActionFailed, "ActionFailed")
}
