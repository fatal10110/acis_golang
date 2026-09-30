package skills

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
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
