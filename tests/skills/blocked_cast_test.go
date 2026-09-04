package skills

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestBlockedCastSendsDistTooFarThenMoveToLocation pins the player CAST
// blocked-arrival arm: DIST_TOO_FAR_CASTING_STOPPED (748) then the base
// same-cell MoveToLocation correction.
func TestBlockedCastSendsDistTooFarThenMoveToLocation(t *testing.T) {
	geo := &gameservertest.GateGeo{}
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

	c.Send(encodeRequestExMagicSkillUseGround(200, 20, 30, 5, false, false))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeMoveToLocation, "cast approach")

	advanced := tickPlayerBlocked(t, srv, objID, geo)
	assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageDistTooFarCastingStopped)
	frame := c.Read()
	if frame[0] == serverpackets.OpcodeStopMove {
		t.Fatalf("blocked CAST opcode = StopMove (%#x), want MoveToLocation (%#x)", frame[0], serverpackets.OpcodeMoveToLocation)
	}
	assertFrameOpcode(t, frame, serverpackets.OpcodeMoveToLocation, "CAST blocked correction")
	objectID, dest, origin := readMoveToLocationCoords(t, frame)
	if objectID != objID {
		t.Fatalf("MoveToLocation object id = %d, want %d", objectID, objID)
	}
	if dest != advanced || origin != advanced {
		t.Fatalf("MoveToLocation dest/origin = %+v/%+v, want advanced cell %+v", dest, origin, advanced)
	}
	drainUntilQuiet(t, c)
}

func tickPlayerBlocked(t *testing.T, srv *gameservertest.Server, objID int32, geo *gameservertest.GateGeo) location.Location {
	t.Helper()
	mover := srv.PlayerMove(t, objID)
	for i := 0; i < 2; i++ {
		if _, moving := mover.UpdatePosition(move.PositionUpdateInterval); !moving {
			t.Fatalf("UpdatePosition() tick %d moving = false, want origin to leave start", i+1)
		}
	}
	advanced := mover.Position()
	geo.Block()
	if _, moving := mover.UpdatePosition(move.PositionUpdateInterval); moving {
		t.Fatal("UpdatePosition() moving = true after path closed, want blocked stop")
	}
	return advanced
}

func readMoveToLocationCoords(t *testing.T, frame []byte) (objectID int32, dest, origin location.Location) {
	t.Helper()
	r := wireReader(frame[1:])
	objectID = r.ReadInt32()
	dest.X = int(r.ReadInt32())
	dest.Y = int(r.ReadInt32())
	dest.Z = int(r.ReadInt32())
	origin.X = int(r.ReadInt32())
	origin.Y = int(r.ReadInt32())
	origin.Z = int(r.ReadInt32())
	if err := r.Err(); err != nil {
		t.Fatalf("read MoveToLocation: %v", err)
	}
	return objectID, dest, origin
}
