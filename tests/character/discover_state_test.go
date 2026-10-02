package character

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestEnteringPlayerSeesRunningPlayerMove pins what a player coming to know
// a running player is shown: CharInfo, then right after it the runner's
// MoveToLocation, from where it stands now to the end of its walk, so the
// runner is not frozen until its next move. A standing player is shown with
// CharInfo alone.
func TestEnteringPlayerSeesRunningPlayerMove(t *testing.T) {
	t.Parallel()
	t.Run("running", func(t *testing.T) {
		srv := gameservertest.Boot(t, gameservertest.WithCharacter("Runner", 1, 0), gameservertest.WithWantChars(1))
		runner := srv.Client
		enterWorld(t, runner)
		drainQuiet(t, runner)
		runnerID := srv.SoleObjectID(t)

		// 4000 units at run speed take far longer than the observer's entry,
		// whichever clock the server runs on.
		target := location.Location{X: spawnOrigin.X + 4000, Y: spawnOrigin.Y, Z: spawnOrigin.Z}
		runner.Send(encodeMoveBackwardToLocation(target, spawnOrigin, 1))
		mustReadOpcode(t, runner, serverpackets.OpcodeMoveToLocation, "runner walk")

		frames := enterSecondPlayer(t, srv)
		next := frameAfterCharInfo(t, frames, runnerID)
		if next == nil || next[0] != serverpackets.OpcodeMoveToLocation {
			t.Fatalf("frame after runner CharInfo = %v, want MoveToLocation", opcodeOf(next))
		}
		objID, dest, origin := gameservertest.ReadMoveToLocationCoords(t, next)
		if objID != runnerID || dest != target {
			t.Fatalf("MoveToLocation object %d to %v, want %d to %v", objID, dest, runnerID, target)
		}
		x, y, z := srv.PlayerPosition(t, runnerID)
		now := location.Location{X: x, Y: y, Z: z}
		if srv.DrivesClock() {
			if origin != now {
				t.Fatalf("MoveToLocation origin = %v, want the runner's position %v", origin, now)
			}
		} else if origin.Y != spawnOrigin.Y || origin.X < spawnOrigin.X || origin.X > now.X {
			t.Fatalf("MoveToLocation origin = %v, want on the walk between %v and %v", origin, spawnOrigin, now)
		}
	})

	t.Run("standing", func(t *testing.T) {
		srv := gameservertest.Boot(t, gameservertest.WithCharacter("Stander", 1, 0), gameservertest.WithWantChars(1))
		enterWorld(t, srv.Client)
		drainQuiet(t, srv.Client)
		standerID := srv.SoleObjectID(t)

		frames := enterSecondPlayer(t, srv)
		if next := frameAfterCharInfo(t, frames, standerID); next != nil && describesObject(next, standerID) {
			t.Fatalf("standing player's CharInfo followed by %#x for it, want CharInfo alone", next[0])
		}
	})
}

// enterSecondPlayer brings a second character into the world at the shared
// spawn and returns every frame its client reads until the server is quiet.
func enterSecondPlayer(t *testing.T, srv *gameservertest.Server) [][]byte {
	t.Helper()
	srv.SeedCharacterFor(t, "player2", "Second", 1, 0)
	c := srv.DialClient(t, "player2", 1)
	c.Send(encodeRequestGameStart(0))
	frames := readUntil(t, c, serverpackets.OpcodeCharSelected)
	c.Send(encodeEnterWorld())
	return append(frames, readUntilQuiet(c)...)
}

// readUntil reads frames up to and including the first with opcode.
func readUntil(t *testing.T, c *testsupport.ScriptedClient, opcode byte) [][]byte {
	t.Helper()
	var frames [][]byte
	for range 100 {
		frame := c.Read()
		frames = append(frames, frame)
		if frame[0] == opcode {
			return frames
		}
	}
	t.Fatalf("no opcode %#x within 100 frames", opcode)
	return nil
}

// frameAfterCharInfo returns the frame read right after objID's CharInfo,
// nil when the CharInfo was the last one.
func frameAfterCharInfo(t *testing.T, frames [][]byte, objID int32) []byte {
	t.Helper()
	for i, frame := range frames {
		if frame[0] != serverpackets.OpcodeCharInfo {
			continue
		}
		r := wire.NewReader(frame[1:])
		r.ReadInt32() // x
		r.ReadInt32() // y
		r.ReadInt32() // z
		r.ReadInt32() // heading
		if r.ReadInt32() != objID {
			continue
		}
		// The player's relation to the viewer follows its CharInfo, ahead
		// of what it is doing (Player.sendInfo, then describeStateToPlayer).
		if i+1 >= len(frames) || frames[i+1][0] != serverpackets.OpcodeRelationChanged || wire.NewReader(frames[i+1][1:]).ReadInt32() != objID {
			t.Fatalf("CharInfo of %d not followed by its RelationChanged", objID)
		}
		if i+2 < len(frames) {
			return frames[i+2]
		}
		return nil
	}
	t.Fatalf("no CharInfo for %d among %d frames", objID, len(frames))
	return nil
}

// describesObject reports whether frame is a MoveToLocation or MagicSkillUse
// for objID.
func describesObject(frame []byte, objID int32) bool {
	if frame[0] != serverpackets.OpcodeMoveToLocation && frame[0] != serverpackets.OpcodeMagicSkillUse {
		return false
	}
	return wire.NewReader(frame[1:]).ReadInt32() == objID
}

func opcodeOf(frame []byte) any {
	if frame == nil {
		return "nothing"
	}
	return frame[0]
}
