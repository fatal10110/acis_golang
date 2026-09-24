package combat

import (
	"encoding/binary"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

const ccStunSkillID = 101

// effectHolder is the live actor surface a crowd-control effect lands on.
type effectHolder interface {
	effect.Actor
	EffectList() *effect.List
	Queue() *sim.Queue
}

// landStun applies a real Stun effect to target on target's own queue, the
// way a landed stun skill does, and waits for its start hook to finish.
func landStun(t *testing.T, target effectHolder) { landEffect(t, target, "Stun") }

// landEffect applies the named real effect to target on target's own queue
// and waits for its start hook to finish.
func landEffect(t *testing.T, target effectHolder, name string) {
	t.Helper()
	e, err := effect.New(
		effect.Skill{ID: ccStunSkillID, Level: 1, Debuff: true},
		modelskill.EffectTemplate{Name: name, Time: 30},
	)
	if err != nil {
		t.Fatalf("effect.New(%s): %v", name, err)
	}
	e.Effector, e.Effected = target, target
	done := make(chan struct{})
	if !target.Queue().Post(func() { target.EffectList().Add(e); close(done) }) {
		t.Fatal("post stun: queue closed")
	}
	<-done
}

// TestStunAbortsPlayerCastInFlight pins a stun landing on a casting player:
// the attack stop answers ActionFailed twice (its idle waits behind the
// running cast), then the cast stop broadcasts MagicSkillCanceled and
// answers ActionFailed, and the aborted cast never launches.
func TestStunAbortsPlayerCastInFlight(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(combatPersistence(t,
			[]modelskill.Definition{{
				ID: 3, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
				HitTime: 5000, ReuseDelay: 60_000, StaticHitTime: true, StaticReuse: true,
				MPInitialConsume: 2, MPConsume: 3, SkillType: "DUMMY",
			}},
		)),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, 3, 1)
	startInWorld(t, c)
	drainUntilQuiet(t, c)

	c.Send(encodeRequestMagicSkillUse(3, false, false))
	assertFrameOpcode(t, mustRead(t, c, "MagicSkillUse"), serverpackets.OpcodeMagicSkillUse, "MagicSkillUse")
	assertFrameOpcode(t, mustRead(t, c, "cast message"), serverpackets.OpcodeSystemMessage, "cast message")
	assertFrameOpcode(t, mustRead(t, c, "SetupGauge"), serverpackets.OpcodeSetupGauge, "SetupGauge")

	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatalf("world.Player(%d) missing", objID)
	}
	player, ok := obj.(interface {
		effectHolder
		CastingNow() bool
	})
	if !ok {
		t.Fatalf("world.Player(%d) = %T is not an effect holder", objID, obj)
	}
	landStun(t, player)

	assertFrameOpcode(t, mustRead(t, c, "idle ActionFailed"), serverpackets.OpcodeActionFailed, "idle ActionFailed")
	assertFrameOpcode(t, mustRead(t, c, "attack-stop ActionFailed"), serverpackets.OpcodeActionFailed, "attack-stop ActionFailed")
	canceled := mustRead(t, c, "MagicSkillCanceled")
	assertFrameOpcode(t, canceled, serverpackets.OpcodeMagicSkillCanceled, "MagicSkillCanceled")
	if got := int32(binary.LittleEndian.Uint32(canceled[1:5])); got != objID {
		t.Fatalf("MagicSkillCanceled object = %d, want %d", got, objID)
	}
	assertFrameOpcode(t, mustRead(t, c, "cast-stop ActionFailed"), serverpackets.OpcodeActionFailed, "cast-stop ActionFailed")

	for {
		frame := c.ReadWithTimeout(readQuietWindow)
		if frame == nil {
			break
		}
		if frame[0] == serverpackets.OpcodeMagicSkillLaunched || frame[0] == serverpackets.OpcodeMagicSkillCanceled {
			t.Fatalf("post-stun frame opcode %#x: the aborted cast must neither launch nor cancel again", frame[0])
		}
	}
	if player.CastingNow() {
		t.Fatal("CastingNow() = true after stun, want the cast aborted")
	}
}

// TestStunStopsWalkingNPC pins a stun landing on a walking monster: its walk
// ends with a StopMove broadcast to observers before anything else.
func TestStunStopsWalkingNPC(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c := srv.Client
	startInWorld(t, c)

	home := location.Location{X: hostileX, Y: hostileY, Z: hostileZ}
	hostile := srv.SpawnMovingHostileNPCAt(t, "Monster", home, home)
	drainUntilQuiet(t, c)

	tickThinkWander(t, hostile)
	assertFrameOpcode(t, mustRead(t, c, "ChangeMoveType"), serverpackets.OpcodeChangeMoveType, "ChangeMoveType")
	assertFrameOpcode(t, mustRead(t, c, "MoveToLocation"), serverpackets.OpcodeMoveToLocation, "MoveToLocation")
	if !hostile.IsMoving() {
		t.Fatal("IsMoving() = false after wander, want a live walk")
	}

	landStun(t, hostile)

	stop := mustRead(t, c, "StopMove")
	assertFrameOpcode(t, stop, serverpackets.OpcodeStopMove, "StopMove")
	if got := int32(binary.LittleEndian.Uint32(stop[1:5])); got != hostile.ObjectID() {
		t.Fatalf("StopMove object = %d, want %d", got, hostile.ObjectID())
	}
	if hostile.IsMoving() {
		t.Fatal("IsMoving() = true after stun, want the walk stopped")
	}
}

// TestRemoveTargetStopsWalkingPlayer pins RemoveTarget landing on a player
// who is walking with nothing selected: the cleared selection answers
// ActionFailed, the attack stop's idle stops the walk (StopMove) and answers
// ActionFailed, and the cast stop answers the last ActionFailed.
func TestRemoveTargetStopsWalkingPlayer(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)
	drainUntilQuiet(t, c)

	c.Send(encodeMoveBackwardToLocation(-2000, 2000, 30))
	assertFrameOpcode(t, mustRead(t, c, "MoveToLocation"), serverpackets.OpcodeMoveToLocation, "MoveToLocation")
	drainUntilQuiet(t, c)

	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatalf("world.Player(%d) missing", objID)
	}
	player, ok := obj.(interface {
		effectHolder
		IsMoving() bool
	})
	if !ok {
		t.Fatalf("world.Player(%d) = %T is not an effect holder", objID, obj)
	}
	if !player.IsMoving() {
		t.Fatal("IsMoving() = false before RemoveTarget, want a live walk")
	}
	landEffect(t, player, "RemoveTarget")

	assertFrameOpcode(t, mustRead(t, c, "unselect ActionFailed"), serverpackets.OpcodeActionFailed, "unselect ActionFailed")
	stop := mustRead(t, c, "StopMove")
	assertFrameOpcode(t, stop, serverpackets.OpcodeStopMove, "StopMove")
	if got := int32(binary.LittleEndian.Uint32(stop[1:5])); got != objID {
		t.Fatalf("StopMove object = %d, want %d", got, objID)
	}
	assertFrameOpcode(t, mustRead(t, c, "attack-stop ActionFailed"), serverpackets.OpcodeActionFailed, "attack-stop ActionFailed")
	assertFrameOpcode(t, mustRead(t, c, "cast-stop ActionFailed"), serverpackets.OpcodeActionFailed, "cast-stop ActionFailed")
	for {
		frame := c.ReadWithTimeout(readQuietWindow)
		if frame == nil {
			break
		}
		if frame[0] == serverpackets.OpcodeMoveToLocation || frame[0] == serverpackets.OpcodeStopMove {
			t.Fatalf("post-RemoveTarget frame opcode %#x: the walk must stay stopped", frame[0])
		}
	}
	if player.IsMoving() {
		t.Fatal("IsMoving() = true after RemoveTarget, want the walk stopped")
	}
}
