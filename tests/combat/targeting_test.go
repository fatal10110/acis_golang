package combat

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// playerOrigin is the class template's spawn point every client stands at.
var playerOrigin = location.Location{X: 10, Y: 20, Z: 30}

// TestActionSelectSendsTargetPackets walks the reference's first-click flow:
// ValidateLocation, MyTargetSelected naming the monster, and the monster's
// full-health StatusUpdate snapshot.
func TestActionSelectSendsTargetPackets(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c := srv.Client
	startInWorld(t, c)
	hostile := srv.SpawnHostileNPC(t)
	drainUntilQuiet(t, c)

	targetHostile(t, c, hostile.ObjectID())
	drainUntilQuiet(t, c)
}

// TestAttackRequestSelectsThenSwingsInRange pins the in-range AttackRequest
// sequence: the first request only selects — ValidateLocation,
// MyTargetSelected, StatusUpdate — and the repeated request attacks:
// AutoAttackStart and Attack go out, and a hit visibly drains the monster.
func TestAttackRequestSelectsThenSwingsInRange(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)
	hostile := srv.SpawnHostileNPCAt(t, location.Location{X: hostileX - 20, Y: hostileY, Z: hostileZ})
	drainUntilQuiet(t, c)

	c.Send(encodeAttackRequest(hostile.ObjectID(), int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))
	assertFrameOpcode(t, mustRead(t, c, "select ValidateLocation"), serverpackets.OpcodeValidateLocation, "select ValidateLocation")
	assertFrameOpcode(t, mustRead(t, c, "MyTargetSelected"), serverpackets.OpcodeMyTargetSelected, "MyTargetSelected")
	assertFrameOpcode(t, mustRead(t, c, "selection StatusUpdate"), serverpackets.OpcodeStatusUpdate, "selection StatusUpdate")
	if reply := c.ReadWithTimeout(readQuietWindow); reply != nil {
		t.Fatalf("first in-range AttackRequest produced opcode %#x, want selection only", reply[0])
	}

	c.Send(encodeAttackRequest(hostile.ObjectID(), int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))
	assertAutoAttackStart(t, c, objID)
	assertAttackBy(t, c, objID)

	waitFor(t, "swing damage", func() bool { return hostile.CurrentHP() < hostile.MaxHP() })
	drainUntilQuiet(t, c)
}

// TestSecondActionClickAttacksSelectedTarget is the unresponsive-attack
// regression: clients attack by plain-clicking twice; the second Action click
// on an in-range target must swing immediately.
func TestSecondActionClickAttacksSelectedTarget(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)
	hostile := srv.SpawnHostileNPCAt(t, location.Location{X: hostileX - 30, Y: hostileY, Z: hostileZ})
	drainUntilQuiet(t, c)

	targetHostile(t, c, hostile.ObjectID())
	c.Send(encodeAction(hostile.ObjectID(), int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))
	assertAutoAttackStart(t, c, objID)
	assertAttackBy(t, c, objID)

	waitFor(t, "second-click swing damage", func() bool { return hostile.CurrentHP() < hostile.MaxHP() })
	drainUntilQuiet(t, c)
}

// TestSecondActionClickWalksTowardDistantTarget covers the out-of-range half
// of the same regression: the second plain click on a far mob must answer
// with MoveToPawn — the walk into range — not silence.
func TestSecondActionClickWalksTowardDistantTarget(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c := srv.Client
	startInWorld(t, c)
	hostile := srv.SpawnHostileNPCAt(t, location.Location{X: hostileX + 500, Y: hostileY, Z: hostileZ})
	drainUntilQuiet(t, c)

	targetHostile(t, c, hostile.ObjectID())
	c.Send(encodeAction(hostile.ObjectID(), int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))
	assertFrameOpcode(t, mustRead(t, c, "MoveToPawn"), serverpackets.OpcodeMoveToPawn, "MoveToPawn")
	drainUntilQuiet(t, c)
}

func sitPlayer(t *testing.T, c *scriptedClient) {
	t.Helper()
	c.Send(encodeRequestChangeWaitType(false))
	assertFrameOpcode(t, mustRead(t, c, "sit ChangeWaitType"), serverpackets.OpcodeChangeWaitType, "sit ChangeWaitType")
}

// TestSittingPlayerCannotAttackSelectedTarget pins the seated attack gate:
// a second Action click on an in-range target answers ActionFailed and does
// not start a swing.
func TestSittingPlayerCannotAttackSelectedTarget(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c := srv.Client
	startInWorld(t, c)
	hostile := srv.SpawnHostileNPCAt(t, location.Location{X: hostileX - 30, Y: hostileY, Z: hostileZ})
	drainUntilQuiet(t, c)

	targetHostile(t, c, hostile.ObjectID())
	sitPlayer(t, c)

	before := hostile.CurrentHP()
	c.Send(encodeAction(hostile.ObjectID(), int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))
	assertFrameOpcode(t, mustRead(t, c, "sitting attack ActionFailed"), serverpackets.OpcodeActionFailed, "sitting attack ActionFailed")
	if reply := c.ReadWithTimeout(readQuietWindow); reply != nil {
		t.Fatalf("sitting attack produced opcode %#x after ActionFailed, want silence", reply[0])
	}
	if got := hostile.CurrentHP(); got != before {
		t.Fatalf("hostile HP = %d after sitting attack, want unchanged %d", got, before)
	}
}

// TestSittingPlayerCannotApproachSelectedTarget pins the seated approach
// gate: a second Action click on a distant target answers ActionFailed and
// does not start MoveToPawn.
func TestSittingPlayerCannotApproachSelectedTarget(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c := srv.Client
	startInWorld(t, c)
	hostile := srv.SpawnHostileNPCAt(t, location.Location{X: hostileX + 500, Y: hostileY, Z: hostileZ})
	drainUntilQuiet(t, c)

	targetHostile(t, c, hostile.ObjectID())
	sitPlayer(t, c)

	c.Send(encodeAction(hostile.ObjectID(), int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))
	assertFrameOpcode(t, mustRead(t, c, "sitting approach ActionFailed"), serverpackets.OpcodeActionFailed, "sitting approach ActionFailed")
	if reply := c.ReadWithTimeout(readQuietWindow); reply != nil {
		t.Fatalf("sitting approach produced opcode %#x after ActionFailed, want silence", reply[0])
	}
}

// TestAttackRequestOnDistantTargetSelectsOnly pins that the first
// AttackRequest on a far target only selects it — no AutoAttackStart until
// the actor is actually in range.
func TestAttackRequestOnDistantTargetSelectsOnly(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c := srv.Client
	startInWorld(t, c)
	hostile := srv.SpawnHostileNPCAt(t, location.Location{X: hostileX + 500, Y: hostileY, Z: hostileZ})
	drainUntilQuiet(t, c)

	c.Send(encodeAttackRequest(hostile.ObjectID(), int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))
	assertFrameOpcode(t, mustRead(t, c, "select ValidateLocation"), serverpackets.OpcodeValidateLocation, "select ValidateLocation")
	assertFrameOpcode(t, mustRead(t, c, "MyTargetSelected"), serverpackets.OpcodeMyTargetSelected, "MyTargetSelected")
	assertFrameOpcode(t, mustRead(t, c, "selection StatusUpdate"), serverpackets.OpcodeStatusUpdate, "selection StatusUpdate")

	if reply := c.ReadWithTimeout(readQuietWindow); reply != nil {
		t.Fatalf("distant AttackRequest produced opcode %#x before arriving in range, want selection only", reply[0])
	}
}

// TestRequestTargetCancelAnswersActionFailed pins the Esc/unselect reply:
// RequestTargetCancel answers ActionFailed after clearing the target.
func TestRequestTargetCancelAnswersActionFailed(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c := srv.Client
	startInWorld(t, c)
	hostile := srv.SpawnHostileNPC(t)
	drainUntilQuiet(t, c)

	targetHostile(t, c, hostile.ObjectID())
	c.Send(encodeRequestTargetCancel(1))
	frame := mustRead(t, c, "cancel reply")
	assertFrameOpcode(t, frame, serverpackets.OpcodeActionFailed, "RequestTargetCancel reply")
	drainUntilQuiet(t, c)
}
