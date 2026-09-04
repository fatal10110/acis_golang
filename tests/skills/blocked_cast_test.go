package skills

import (
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestBlockedCastSendsDistTooFarThenMoveToLocation pins the player CAST
// blocked-arrival arm: DIST_TOO_FAR_CASTING_STOPPED (748) then the base
// same-cell MoveToLocation correction.
func TestBlockedCastSendsDistTooFarThenMoveToLocation(t *testing.T) {
	geo := &gameservertest.GateGeo{}
	srv := bootBlockedGroundCast(t, geo)
	c, objID := srv.Client, srv.SoleObjectID(t)

	c.Send(encodeRequestExMagicSkillUseGround(200, 20, 30, 5, false, false))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeMoveToLocation, "cast approach")

	advanced := srv.TickPlayerBlocked(t, objID, geo)
	assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageDistTooFarCastingStopped)
	frame := c.Read()
	if frame[0] == serverpackets.OpcodeStopMove {
		t.Fatalf("blocked CAST opcode = StopMove (%#x), want MoveToLocation (%#x)", frame[0], serverpackets.OpcodeMoveToLocation)
	}
	assertFrameOpcode(t, frame, serverpackets.OpcodeMoveToLocation, "CAST blocked correction")
	objectID, dest, origin := gameservertest.ReadMoveToLocationCoords(t, frame)
	if objectID != objID {
		t.Fatalf("MoveToLocation object id = %d, want %d", objectID, objID)
	}
	if dest != advanced || origin != advanced {
		t.Fatalf("MoveToLocation dest/origin = %+v/%+v, want advanced cell %+v", dest, origin, advanced)
	}
	drainUntilQuiet(t, c)
}

// TestBlockedWalkAfterGroundCastBroadcastsMoveToLocation pins that a later
// client walk replaces the parked CAST slot: blocked arrival is MOVE_TO
// (same-cell MoveToLocation only), not DIST_TOO_FAR_CASTING_STOPPED.
func TestBlockedWalkAfterGroundCastBroadcastsMoveToLocation(t *testing.T) {
	geo := &gameservertest.GateGeo{}
	srv := bootBlockedGroundCast(t, geo)
	c, objID := srv.Client, srv.SoleObjectID(t)

	c.Send(encodeRequestExMagicSkillUseGround(200, 20, 30, 5, false, false))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeMoveToLocation, "cast approach")

	c.Send(encodeMoveBackwardToLocation(80, 20, 30))
	drainUntilQuiet(t, c)

	advanced := srv.TickPlayerBlocked(t, objID, geo)
	frame := c.Read()
	if frame[0] == serverpackets.OpcodeSystemMessage {
		t.Fatalf("MOVE_TO after CAST approach sent SystemMessage, want MoveToLocation")
	}
	if frame[0] == serverpackets.OpcodeStopMove {
		t.Fatalf("MOVE_TO after CAST approach sent StopMove, want MoveToLocation")
	}
	assertFrameOpcode(t, frame, serverpackets.OpcodeMoveToLocation, "MOVE_TO blocked correction")
	objectID, dest, origin := gameservertest.ReadMoveToLocationCoords(t, frame)
	if objectID != objID {
		t.Fatalf("MoveToLocation object id = %d, want %d", objectID, objID)
	}
	if dest != advanced || origin != advanced {
		t.Fatalf("MoveToLocation dest/origin = %+v/%+v, want advanced cell %+v", dest, origin, advanced)
	}
	drainUntilQuiet(t, c)
}

func bootBlockedGroundCast(t *testing.T, geo *gameservertest.GateGeo) *gameservertest.Server {
	t.Helper()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithGeo(geo),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{{
			ID: 5, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetGround,
			CastRange: 40, HitTime: 500, StaticHitTime: true, SkillType: "BUFF",
			Effects: []modelskill.EffectTemplate{{Name: "Buff", Time: 60, Icon: true}},
		}})),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, 5, 1)
	startInWorld(t, c)
	return srv
}
