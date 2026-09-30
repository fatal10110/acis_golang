package skills

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference for the tests below (#2925): a THINK with CAST current runs
// PlayerAI.thinkCast (PlayerAI.java:219-): a player that cannot act or has
// every skill disabled goes idle (doIdleIntention, whose thinkIdle stops the
// walk, CreatureAI.java:209-212) with ActionFailed; otherwise a GROUND skill
// out of range walks to the signet again (CreatureMove.maybeMoveToLocation,
// CreatureMove.java:433-447, a fresh MoveToLocation from where the player
// stands) and casts once in range. The one effect exit that thinks a player
// is ImmobileUntilAttacked's (EffectImmobileUntilAttacked.java:41,53); its
// start (abortAll, PlayableAttack.stop -> PlayableAI.tryToIdle) idles an
// approach unless the AI was already denied, and every denying effect's own
// start idles it too, so the tests drive the THINK straight onto the
// player's queue with the approach parked.

// thinkCastEffectSkillID owns the ImmobileUntilAttacked effect landed below.
const thinkCastEffectSkillID = 4101

// thinkCastHolder is the live player surface an effect lands on.
type thinkCastHolder interface {
	effect.Actor
	EffectList() *effect.List
}

func thinkCastHolderOf(t *testing.T, srv *gameservertest.Server, objID int32) thinkCastHolder {
	t.Helper()
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatalf("world.Player(%d) missing", objID)
	}
	holder, ok := obj.(thinkCastHolder)
	if !ok {
		t.Fatalf("world.Player(%d) = %T is not an effect holder", objID, obj)
	}
	return holder
}

// countFrames counts the frames in log with opcode.
func countFrames(log frameLog, opcode byte) int {
	n := 0
	for _, frame := range log {
		if frame[0] == opcode {
			n++
		}
	}
	return n
}

// TestThinkMidGroundCastApproachWalksOnAfresh: a THINK while a ground-cast
// approach is current walks to the signet afresh from where the player
// stands, answering no ActionFailed and casting nothing yet, and the
// arrival casts it once.
func TestThinkMidGroundCastApproachWalksOnAfresh(t *testing.T) {
	t.Parallel()
	srv, objID, _ := bootIntentionCaster(t, modelskill.TargetGround)
	c := srv.Client
	dest := startGroundCastApproach(t, srv, objID)
	if srv.DrivesClock() {
		srv.Advance(t, 500*time.Millisecond)
	}
	drainUntilQuiet(t, c)

	onPlayerQueue(t, srv, objID, func(pc *player.Character) { pc.WakeAI() })
	srv.Settle(t)
	ax, ay, az := srv.PlayerPosition(t, objID)
	log := readFrameLog(c)
	rewalk := log.index(isOpcode(serverpackets.OpcodeMoveToLocation))
	if rewalk < 0 {
		t.Fatal("no fresh MoveToLocation for the THINK on the ground-cast approach")
	}
	objectID, gotDest, origin := gameservertest.ReadMoveToLocationCoords(t, log[rewalk])
	if objectID != objID || gotDest != dest {
		t.Fatalf("fresh MoveToLocation object/dest = %d/%+v, want %d/%+v", objectID, gotDest, objID, dest)
	}
	if srv.DrivesClock() {
		if want := (location.Location{X: ax, Y: ay, Z: az}); origin != want {
			t.Fatalf("fresh MoveToLocation origin = %+v, want the current position %+v", origin, want)
		}
	}
	if at := log.index(isOpcode(serverpackets.OpcodeActionFailed)); at >= 0 {
		t.Fatalf("ActionFailed at frame %d: a re-thought cast approach answers none", at)
	}
	if at := log.index(isOpcode(serverpackets.OpcodeMagicSkillUse)); at >= 0 && at < rewalk {
		t.Fatalf("MagicSkillUse at frame %d ahead of the fresh walk", at)
	}

	waitForPlayerPosition(t, srv, objID, dest)
	log = append(log, readFrameLog(c)...)
	if n := countFrames(log, serverpackets.OpcodeMagicSkillUse); n != 1 {
		t.Fatalf("MagicSkillUse frames = %d, want 1: the arrival casts the approach once", n)
	}
}

// TestThinkDeniedGroundCastApproachGoesIdle: a THINK finding the player
// unable to act idles the approach: the walk stops, ActionFailed answers it,
// and a later THINK neither walks nor casts it again.
func TestThinkDeniedGroundCastApproachGoesIdle(t *testing.T) {
	t.Parallel()
	srv, objID, _ := bootIntentionCaster(t, modelskill.TargetGround)
	c := srv.Client
	startGroundCastApproach(t, srv, objID)
	drainUntilQuiet(t, c)

	onPlayerQueue(t, srv, objID, func(pc *player.Character) {
		pc.SetParalyzed(true)
		pc.WakeAI()
	})
	srv.Settle(t)
	log := readFrameLog(c)
	if at := log.index(isOpcode(serverpackets.OpcodeActionFailed)); at < 0 {
		t.Fatal("no ActionFailed for the THINK on a paralyzed player's cast approach")
	}
	if at := log.index(isOpcode(serverpackets.OpcodeStopMove)); at < 0 {
		t.Fatal("no StopMove: the idled cast approach kept walking")
	}
	if at := log.index(isOpcode(serverpackets.OpcodeMoveToLocation)); at >= 0 {
		t.Fatalf("MoveToLocation at frame %d for a paralyzed player's THINK", at)
	}

	onPlayerQueue(t, srv, objID, func(pc *player.Character) {
		pc.SetParalyzed(false)
		pc.WakeAI()
	})
	srv.Settle(t)
	if srv.DrivesClock() {
		srv.Advance(t, 5*time.Second)
	}
	log = readFrameLog(c)
	if at := log.index(isOpcode(serverpackets.OpcodeMoveToLocation)); at >= 0 {
		t.Fatalf("MoveToLocation at frame %d: the idled approach walked again", at)
	}
	if at := log.index(isOpcode(serverpackets.OpcodeMagicSkillUse)); at >= 0 {
		t.Fatalf("MagicSkillUse at frame %d: the idled approach cast", at)
	}
}

// TestImmobileUntilAttackedIdlesGroundCastApproach pins the reachability the
// THINK depends on: ImmobileUntilAttacked landing on a player free to act
// idles the approach, so its exit THINK finds nothing to walk or cast.
func TestImmobileUntilAttackedIdlesGroundCastApproach(t *testing.T) {
	t.Parallel()
	srv, objID, _ := bootIntentionCaster(t, modelskill.TargetGround)
	c := srv.Client
	startGroundCastApproach(t, srv, objID)
	drainUntilQuiet(t, c)

	e, err := effect.New(effect.Skill{ID: thinkCastEffectSkillID, Level: 1, Debuff: true},
		modelskill.EffectTemplate{Name: "ImmobileUntilAttacked", Time: 60})
	if err != nil {
		t.Fatalf("effect.New(ImmobileUntilAttacked): %v", err)
	}
	onPlayerQueue(t, srv, objID, func(*player.Character) {
		holder := thinkCastHolderOf(t, srv, objID)
		e.Effector, e.Effected = holder, holder
		holder.EffectList().Add(e)
	})
	srv.Settle(t)
	drainUntilQuiet(t, c)
	onPlayerQueue(t, srv, objID, func(*player.Character) {
		thinkCastHolderOf(t, srv, objID).EffectList().Remove(e)
	})
	srv.Settle(t)
	if srv.DrivesClock() {
		srv.Advance(t, 5*time.Second)
	}
	log := readFrameLog(c)
	if at := log.index(isOpcode(serverpackets.OpcodeMoveToLocation)); at >= 0 {
		t.Fatalf("MoveToLocation at frame %d: the exit THINK walked an idled approach", at)
	}
	if at := log.index(isOpcode(serverpackets.OpcodeMagicSkillUse)); at >= 0 {
		t.Fatalf("MagicSkillUse at frame %d: the exit THINK cast an idled approach", at)
	}
}
