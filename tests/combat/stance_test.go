package combat

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
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

const stanceTimeoutDummySkill = 3

// TestAttackStanceTimeoutSendsAutoAttackStopWithoutStoppingCast pins the
// inactivity expiry path: after 15s the known recipients get AutoAttackStop,
// but livePlayer.Stop is not invoked — an in-flight cast still launches
// instead of MagicSkillCanceled. Combat and cubic runtimes share that Stop
// method, so a surviving cast is the same proof they were left running.
func TestAttackStanceTimeoutSendsAutoAttackStopWithoutStoppingCast(t *testing.T) {
	var nowMS atomic.Int64
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithAttackStanceClock(func() time.Time { return time.UnixMilli(nowMS.Load()) }),
		gameservertest.WithSkills(combatPersistence(t,
			[]modelskill.Definition{{
				ID: stanceTimeoutDummySkill, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
				HitTime: 4000, ReuseDelay: 60_000, StaticHitTime: true, StaticReuse: true,
				MPInitialConsume: 0, MPConsume: 0, SkillType: "DUMMY",
			}},
		)),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, stanceTimeoutDummySkill, 1)
	startInWorld(t, c)
	hostile := srv.SpawnHostileNPCAt(t, location.Location{X: hostileX - 20, Y: hostileY, Z: hostileZ})
	drainUntilQuiet(t, c)

	targetHostile(t, c, hostile.ObjectID())
	c.Send(encodeAction(hostile.ObjectID(), int32(playerOrigin.X), int32(playerOrigin.Y), int32(playerOrigin.Z), false))
	assertAutoAttackStart(t, c, objID)
	waitFor(t, "opening swing", func() bool { return hostile.CurrentHP() < hostile.MaxHP() })

	// Walk away: the swing loop stops but the combat-stance tracker stays
	// registered until inactivity expiry, matching the restart/logout test.
	c.Send(encodeMoveBackwardToLocation(-2000, 2000, 30))
	drainUntilQuiet(t, c)
	// The last swing's AttackingNow flag can outlive the walk; casting
	// during that window is deferred with ActionFailed and may never start
	// if the swing is cancelled instead of finishing.
	time.Sleep(2 * time.Second)
	drainUntilQuiet(t, c)

	c.Send(encodeRequestMagicSkillUse(stanceTimeoutDummySkill, false, false))
	assertFrameOpcode(t, mustRead(t, c, "cast stop"), serverpackets.OpcodeStopMove, "cast stop")
	assertFrameOpcode(t, mustRead(t, c, "MagicSkillUse"), serverpackets.OpcodeMagicSkillUse, "MagicSkillUse")
	assertFrameOpcode(t, mustRead(t, c, "cast message"), serverpackets.OpcodeSystemMessage, "cast message")
	assertFrameOpcode(t, mustRead(t, c, "SetupGauge"), serverpackets.OpcodeSetupGauge, "SetupGauge")

	if !playerCastingNow(t, srv, objID) {
		t.Fatal("cast was not in flight before stance timeout")
	}

	nowMS.Add(task.AttackStancePeriod.Milliseconds())
	if err := srv.AttackStance.Tick(); err != nil {
		t.Fatalf("AttackStance.Tick() = %v", err)
	}

	stop := readSkippingCombat(t, c, serverpackets.OpcodeAutoAttackStop, "timeout AutoAttackStop")
	if got := wireReader(stop[1:]).ReadInt32(); got != objID {
		t.Fatalf("AutoAttackStop object id = %d, want %d", got, objID)
	}
	if !playerCastingNow(t, srv, objID) {
		t.Fatal("stance timeout stopped the in-flight cast")
	}
	if srv.AttackStance.InAttackStance(worldActor{id: objID}) {
		t.Fatal("timeout left the actor in the stance tracker")
	}

	assertFrameOpcode(t, mustRead(t, c, "MagicSkillLaunched"), serverpackets.OpcodeMagicSkillLaunched, "MagicSkillLaunched")
}

type worldActor struct{ id int32 }

func (a worldActor) ObjectID() int32 { return a.id }

func playerCastingNow(t *testing.T, srv *gameservertest.Server, objID int32) bool {
	t.Helper()
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatalf("world.Player(%d) missing", objID)
	}
	caster, ok := obj.(interface{ CastingNow() bool })
	if !ok {
		t.Fatalf("world.Player(%d) = %T does not expose CastingNow", objID, obj)
	}
	return caster.CastingNow()
}

// readSkippingCombat reads until opcode want, skipping in-flight Attack
// and StatusUpdate frames. MagicSkillCanceled is a failure: the timeout
// must not abort a live cast.
func readSkippingCombat(t *testing.T, c *scriptedClient, want byte, what string) []byte {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		frame := c.ReadWithTimeout(200 * time.Millisecond)
		if frame == nil {
			continue
		}
		switch frame[0] {
		case want:
			return frame
		case serverpackets.OpcodeMagicSkillCanceled:
			t.Fatalf("%s: MagicSkillCanceled, want %#x (timeout must not abort the cast)", what, want)
		case serverpackets.OpcodeAttack, serverpackets.OpcodeStatusUpdate, serverpackets.OpcodeSetupGauge:
			continue
		default:
			t.Fatalf("%s opcode = %#x, want %#x", what, frame[0], want)
		}
	}
	t.Fatalf("%s never arrived", what)
	return nil
}
