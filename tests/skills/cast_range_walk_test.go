package skills

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// Cast-range approach fixtures: a ONE-target nuke with a 300 cast range and
// a self buff, both paid in MP.
const (
	rangeWalkNukeID  = 42
	rangeWalkBuffID  = 43
	rangeWalkRange   = 300
	rangeWalkHitTime = 500
)

// rangeWalkCaster boots a caster who knows the nuke and the buff, in world,
// with a monster spawned dx units east of it and selected.
func rangeWalkCaster(t *testing.T, dx int) (srv *gameservertest.Server, c *testsupport.ScriptedClient, objID int32, origin, at location.Location, hostileID int32) {
	t.Helper()
	srv = gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{
			{
				ID: rangeWalkNukeID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
				CastRange: rangeWalkRange, HitTime: rangeWalkHitTime, ReuseDelay: 60_000, StaticHitTime: true, StaticReuse: true,
				MPConsume: 5, SkillType: "PDAM", Power: 1,
			},
			{
				ID: rangeWalkBuffID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
				HitTime: rangeWalkHitTime, ReuseDelay: 60_000, StaticHitTime: true, StaticReuse: true,
				MPConsume: 3, SkillType: "BUFF",
				Effects: []modelskill.EffectTemplate{{Name: "Buff", Time: 60, Icon: true}},
			},
		})),
	)
	c, objID = srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, rangeWalkNukeID, 1)
	seedKnownSkill(t, srv, objID, rangeWalkBuffID, 1)
	startInWorld(t, c)
	x, y, z := srv.PlayerPosition(t, objID)
	origin = location.Location{X: x, Y: y, Z: z}
	at = location.Location{X: x + dx, Y: y, Z: z}
	hostile := srv.SpawnHostileNPCAt(t, at)
	drainUntilQuiet(t, c)
	c.Send(encodeAction(hostile.ObjectID(), int32(at.X), int32(at.Y), int32(at.Z), false))
	drainUntilQuiet(t, c)
	return srv, c, objID, origin, at, hostile.ObjectID()
}

// assertMoveToPawn checks frame is the player's MoveToPawn toward targetID
// at distance, from origin.
func assertMoveToPawn(t *testing.T, frame []byte, objID, targetID int32, distance int, origin location.Location) {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeMoveToPawn, "cast approach MoveToPawn")
	r := wireReader(frame[1:])
	gotObj, gotTarget, gotDistance := r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
	gotOrigin := location.Location{X: int(r.ReadInt32()), Y: int(r.ReadInt32()), Z: int(r.ReadInt32())}
	if gotObj != objID || gotTarget != targetID || gotDistance != int32(distance) || gotOrigin != origin {
		t.Fatalf("MoveToPawn = object %d target %d distance %d origin %+v, want %d/%d/%d/%+v",
			gotObj, gotTarget, gotDistance, gotOrigin, objID, targetID, distance, origin)
	}
}

// isMagicSkillUseOf matches the player's MagicSkillUse of skillID.
func isMagicSkillUseOf(objID, skillID int32) func([]byte) bool {
	return func(frame []byte) bool {
		if frame[0] != serverpackets.OpcodeMagicSkillUse {
			return false
		}
		r := wireReader(frame[1:])
		caster, _, id := r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
		return caster == objID && id == skillID
	}
}

// TestCastOutOfRangeWalksThenCasts pins PlayerAI.thinkCast's approach
// (PlayerAI.java:259-271, PlayerMove.maybeMoveToPawn PlayerMove.java:
// 335-352): a ONE-target skill on a monster beyond its cast range plus both
// footprints walks to it with MoveToPawn at the cast range, paying nothing,
// and the arrival thinks the CAST again, which casts from there.
func TestCastOutOfRangeWalksThenCasts(t *testing.T) {
	t.Parallel()
	srv, c, objID, origin, at, hostileID := rangeWalkCaster(t, 800)
	mpBefore := srv.PlayerCurrentMP(t, objID)

	c.Send(encodeRequestMagicSkillUse(rangeWalkNukeID, false, false))
	assertMoveToPawn(t, c.Read(), objID, hostileID, rangeWalkRange, origin)
	if srv.PlayerCastingNow(t, objID) {
		t.Fatal("casting before the approach walk arrived")
	}
	if got := srv.PlayerCurrentMP(t, objID); got != mpBefore {
		t.Fatalf("MP after the approach started = %d, want %d untouched", got, mpBefore)
	}

	srv.AdvanceUntil(t, "cast on arrival", func() bool { return srv.PlayerCastingNow(t, objID) })
	log := readFrameLog(c)
	cast := log.index(isMagicSkillUseOf(objID, rangeWalkNukeID))
	if cast < 0 {
		t.Fatal("no MagicSkillUse once the approach walk arrived")
	}
	r := wireReader(log[cast][1:])
	if _, target := r.ReadInt32(), r.ReadInt32(); target != hostileID {
		t.Fatalf("MagicSkillUse target = %d, want the monster %d", target, hostileID)
	}
	x, y, z := srv.PlayerPosition(t, objID)
	if !location.In3DRadius(x, y, z, at.X, at.Y, at.Z, rangeWalkRange+100) {
		t.Fatalf("cast started at (%d,%d,%d), want within cast range of the monster at %+v", x, y, z, at)
	}
}

// TestShiftCastOutOfRangeRefusedTooFar pins the shift-held half of
// PlayerAI.thinkCast (PlayerAI.java:261-270): out of range it answers
// TARGET_TOO_FAR and goes idle, with no MoveToPawn, no MagicSkillUse and no
// MP spent.
func TestShiftCastOutOfRangeRefusedTooFar(t *testing.T) {
	t.Parallel()
	srv, c, objID, origin, _, _ := rangeWalkCaster(t, 800)
	mpBefore := srv.PlayerCurrentMP(t, objID)

	c.Send(encodeRequestMagicSkillUse(rangeWalkNukeID, false, true))
	assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageTargetTooFar)
	srv.Advance(t, 2*time.Second)
	for _, frame := range readFrameLog(c) {
		if frame[0] == serverpackets.OpcodeMoveToPawn || frame[0] == serverpackets.OpcodeMoveToLocation || frame[0] == serverpackets.OpcodeMagicSkillUse {
			t.Fatalf("frame %#x after the shift-held refusal, want no walk and no cast", frame[0])
		}
	}
	if srv.PlayerCastingNow(t, objID) {
		t.Fatal("casting after TARGET_TOO_FAR")
	}
	if got := srv.PlayerCurrentMP(t, objID); got != mpBefore {
		t.Fatalf("MP after TARGET_TOO_FAR = %d, want %d untouched", got, mpBefore)
	}
	if x, y, z := srv.PlayerPosition(t, objID); (location.Location{X: x, Y: y, Z: z}) != origin {
		t.Fatalf("player at (%d,%d,%d) after TARGET_TOO_FAR, want still at %+v", x, y, z, origin)
	}
}

// TestCastInRangeStartsAtOnce is the in-range control, shift held or not:
// within cast range plus both footprints the cast starts where the player
// stands, with no approach.
func TestCastInRangeStartsAtOnce(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		shift bool
	}{{"plain", false}, {"shift", true}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv, c, objID, _, _, hostileID := rangeWalkCaster(t, rangeWalkRange)
			c.Send(encodeRequestMagicSkillUse(rangeWalkNukeID, false, tc.shift))
			readCastStartFrames(t, c, objID, rangeWalkNukeID, 1, rangeWalkHitTime, 60_000, hostileID)
			if !srv.PlayerCastingNow(t, objID) {
				t.Fatal("not casting after an in-range request")
			}
		})
	}
}

// TestLaterCastReplacesCastApproach pins the approach as the CAST intention:
// a later skill request that casts at once replaces it (AbstractAI.
// doCastIntention), so the walk's target is never cast on once that later
// cast ends.
func TestLaterCastReplacesCastApproach(t *testing.T) {
	t.Parallel()
	srv, c, objID, _, _, _ := rangeWalkCaster(t, 800)

	c.Send(encodeRequestMagicSkillUse(rangeWalkNukeID, false, false))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeMoveToPawn, "cast approach")
	c.Send(encodeRequestMagicSkillUse(rangeWalkBuffID, false, false))
	srv.AdvanceUntil(t, "buff cast", func() bool { return srv.PlayerCastingNow(t, objID) })
	srv.AdvanceUntil(t, "buff cast end", func() bool { return !srv.PlayerCastingNow(t, objID) })
	srv.Advance(t, 10*time.Second)

	log := readFrameLog(c)
	if log.index(isMagicSkillUseOf(objID, rangeWalkBuffID)) < 0 {
		t.Fatal("no MagicSkillUse for the later buff")
	}
	if at := log.index(isMagicSkillUseOf(objID, rangeWalkNukeID)); at >= 0 {
		t.Fatalf("MagicSkillUse of the replaced approach's skill at frame %d", at)
	}
}

// TestMoveReplacesCastApproach pins a click-to-move replacing the approach:
// the new walk's arrival casts nothing.
func TestMoveReplacesCastApproach(t *testing.T) {
	t.Parallel()
	srv, c, objID, origin, _, _ := rangeWalkCaster(t, 800)

	c.Send(encodeRequestMagicSkillUse(rangeWalkNukeID, false, false))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeMoveToPawn, "cast approach")
	dest := location.Location{X: origin.X, Y: origin.Y + 100, Z: origin.Z}
	c.Send(encodeMoveBackwardToLocation(int32(dest.X), int32(dest.Y), int32(dest.Z)))
	srv.Advance(t, 10*time.Second)

	log := readFrameLog(c)
	if at := log.index(isMagicSkillUseOf(objID, rangeWalkNukeID)); at >= 0 {
		t.Fatalf("MagicSkillUse at frame %d after a move replaced the approach", at)
	}
	if srv.PlayerCastingNow(t, objID) {
		t.Fatal("casting after a move replaced the approach")
	}
}

// TestRootedCastOutOfRangeAnsweredActionFailed pins the immobile caster: a
// rooted player casting the nuke on a monster beyond cast range is answered
// ActionFailed and walks nowhere, casts nothing and pays nothing. The
// reference (PlayerMove.maybeMoveToPawn with isMovementDisabled) returns
// with no packet; the Go server still answers the request.
func TestRootedCastOutOfRangeAnsweredActionFailed(t *testing.T) {
	t.Parallel()
	srv, c, objID, origin, _, _ := rangeWalkCaster(t, 800)
	onPlayerQueue(t, srv, objID, func(pc *player.Character) {
		root, err := effect.New(effect.Skill{ID: 101, Level: 1, Debuff: true}, modelskill.EffectTemplate{Name: "Root", Time: 30})
		if err != nil {
			t.Errorf("new root effect: %v", err)
			return
		}
		root.Effector, root.Effected = pc, pc
		pc.EffectList().Add(root)
	})
	drainUntilQuiet(t, c)
	onPlayerQueue(t, srv, objID, func(pc *player.Character) {
		if !pc.MovementDisabled() {
			t.Error("MovementDisabled() = false after Root landed, want true")
		}
	})
	mpBefore := srv.PlayerCurrentMP(t, objID)

	c.Send(encodeRequestMagicSkillUse(rangeWalkNukeID, false, false))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeActionFailed, "rooted out-of-range cast")
	srv.Advance(t, 10*time.Second)
	for _, frame := range readFrameLog(c) {
		if frame[0] == serverpackets.OpcodeMoveToPawn || frame[0] == serverpackets.OpcodeMoveToLocation || frame[0] == serverpackets.OpcodeMagicSkillUse {
			t.Fatalf("frame %#x after the rooted refusal, want no walk and no cast", frame[0])
		}
	}
	if srv.PlayerCastingNow(t, objID) {
		t.Fatal("casting after the rooted refusal")
	}
	if got := srv.PlayerCurrentMP(t, objID); got != mpBefore {
		t.Fatalf("MP after the rooted refusal = %d, want %d untouched", got, mpBefore)
	}
	if x, y, z := srv.PlayerPosition(t, objID); (location.Location{X: x, Y: y, Z: z}) != origin {
		t.Fatalf("player at (%d,%d,%d) after the rooted refusal, want still at %+v", x, y, z, origin)
	}
}

// TestShiftCastMidApproachStopsWalk pins the idle half of the shift-held
// refusal (PlayerAI.thinkCast into CreatureAI.thinkIdle's move stop): the
// same nuke sent with shift while its approach walk is under way answers
// TARGET_TOO_FAR, stops the walk where the player stands, and the dropped
// approach casts nothing once its old arrival time has passed.
func TestShiftCastMidApproachStopsWalk(t *testing.T) {
	t.Parallel()
	srv, c, objID, origin, _, _ := rangeWalkCaster(t, 800)
	mpBefore := srv.PlayerCurrentMP(t, objID)

	c.Send(encodeRequestMagicSkillUse(rangeWalkNukeID, false, false))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeMoveToPawn, "cast approach")
	tickPlayerWalk(t, srv, objID)
	drainUntilQuiet(t, c)

	c.Send(encodeRequestMagicSkillUse(rangeWalkNukeID, false, true))
	log := assertShiftRefusalStopsWalk(t, srv, c, objID, origin)
	if at := log.index(isMagicSkillUseOf(objID, rangeWalkNukeID)); at >= 0 {
		t.Fatalf("MagicSkillUse at frame %d after the shift-held refusal dropped the approach", at)
	}
	if srv.PlayerCastingNow(t, objID) {
		t.Fatal("casting after the shift-held refusal dropped the approach")
	}
	if got := srv.PlayerCurrentMP(t, objID); got != mpBefore {
		t.Fatalf("MP after the shift-held refusal = %d, want %d untouched", got, mpBefore)
	}
}

// TestShiftGroundCastMidApproachStopsWalk is TestShiftCastMidApproachStopsWalk
// for the GROUND branch: a shift-held signet request while the approach to
// the signet point is under way stops the walk and casts nothing.
func TestShiftGroundCastMidApproachStopsWalk(t *testing.T) {
	t.Parallel()
	srv := bootBlockedGroundCast(t, &gameservertest.GateGeo{})
	c, objID := srv.Client, srv.SoleObjectID(t)
	x, y, z := srv.PlayerPosition(t, objID)
	origin := location.Location{X: x, Y: y, Z: z}

	c.Send(encodeRequestExMagicSkillUseGround(int32(x+800), int32(y), int32(z), 5, false, false))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeMoveToLocation, "ground-cast approach")
	tickPlayerWalk(t, srv, objID)
	drainUntilQuiet(t, c)

	c.Send(encodeRequestExMagicSkillUseGround(int32(x+800), int32(y), int32(z), 5, false, true))
	log := assertShiftRefusalStopsWalk(t, srv, c, objID, origin)
	if at := log.index(func(frame []byte) bool { return frame[0] == serverpackets.OpcodeMagicSkillUse }); at >= 0 {
		t.Fatalf("MagicSkillUse at frame %d after the shift-held refusal dropped the approach", at)
	}
	if srv.PlayerCastingNow(t, objID) {
		t.Fatal("casting after the shift-held refusal dropped the approach")
	}
}

// tickPlayerWalk moves the player's walk under way two interpolation ticks
// along its path.
func tickPlayerWalk(t *testing.T, srv *gameservertest.Server, objID int32) {
	t.Helper()
	mover := srv.PlayerMove(t, objID)
	for i := range 2 {
		if _, moving := mover.UpdatePosition(move.PositionUpdateInterval); !moving {
			t.Fatalf("UpdatePosition() tick %d moving = false, want the approach under way", i+1)
		}
	}
}

// assertShiftRefusalStopsWalk reads TARGET_TOO_FAR then the player's
// StopMove, checks the player stopped short of where it started walking to
// and stays there past the old arrival time, walking nowhere. It returns the
// frames sent over that time.
func assertShiftRefusalStopsWalk(t *testing.T, srv *gameservertest.Server, c *testsupport.ScriptedClient, objID int32, origin location.Location) frameLog {
	t.Helper()
	assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageTargetTooFar)
	stop := c.Read()
	assertFrameOpcode(t, stop, serverpackets.OpcodeStopMove, "shift-held refusal StopMove")
	if id := wireReader(stop[1:]).ReadInt32(); id != objID {
		t.Fatalf("StopMove object = %d, want the player %d", id, objID)
	}
	mover := srv.PlayerMove(t, objID)
	stopped := mover.Position()
	if stopped == origin {
		t.Fatalf("player still at %+v, want the approach to have left it before the refusal", origin)
	}
	if mover.Moving() {
		t.Fatal("still walking after the shift-held refusal")
	}
	srv.Advance(t, 10*time.Second)
	if got := mover.Position(); got != stopped {
		t.Fatalf("player at %+v after the old arrival time, want stopped at %+v", got, stopped)
	}
	log := readFrameLog(c)
	for _, frame := range log {
		if frame[0] == serverpackets.OpcodeMoveToPawn || frame[0] == serverpackets.OpcodeMoveToLocation || frame[0] == serverpackets.OpcodeStopMove {
			t.Fatalf("walk frame %#x after the shift-held refusal's StopMove, want none", frame[0])
		}
	}
	return log
}
