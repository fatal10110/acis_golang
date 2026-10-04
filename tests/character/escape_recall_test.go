package character

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/restart"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// escapeTown is a restart table whose one point covers the fixture spawn's
// map region and lands at town.
func escapeTown(town location.Location) *restart.Table {
	region := location.Point{
		X: (0-world.MinX)/world.TileSize + world.TileXMin,
		Y: (0-world.MinY)/world.TileSize + world.TileYMin,
	}
	return &restart.Table{Points: []restart.Point{{
		Name: "TestTown", LocName: 910, MapRegions: []location.Point{region},
		Points: []location.Location{town},
	}}}
}

// TestEscapeCommandRecallsToTown pins /unstuck's end (Escape: 5 minutes,
// 2099, a RECALL skill: L2SkillTeleport with no teleCoords and no
// recallType): five minutes after the command, the player is taken to its
// nearest town restart point within 20 of it. The teleport aborts the cast
// it ends, as Creature.teleportTo's abortAll does in the reference: the
// cast's MagicSkillCanceled goes out ahead of the TeleportToLocation.
func TestEscapeCommandRecallsToTown(t *testing.T) {
	t.Parallel()
	town := location.Location{X: -5000, Y: 2000, Z: 30}
	srv, objID := bootUserCommands(t,
		gameservertest.WithSkills(escapeSkills(t)),
		gameservertest.WithRestartPoints(escapeTown(town)),
	)
	userCommandFrames(t, srv.Client, cmdEscape)

	srv.Advance(t, 299*time.Second)
	if x, y, _ := srv.PlayerPosition(t, objID); x < town.X+1000 {
		t.Fatalf("player at %d, %d before the cast's end, want it still home", x, y)
	}
	srv.AdvanceUntil(t, "escape recall", func() bool {
		x, y, _ := srv.PlayerPosition(t, objID)
		return x >= town.X-20 && x <= town.X+20 && y >= town.Y-20 && y <= town.Y+20
	})

	frames := readUntilQuiet(srv.Client)
	teleport := firstOpcode(frames, serverpackets.OpcodeTeleportToLocation)
	if teleport < 0 {
		t.Fatalf("recall frames = %x, want a TeleportToLocation", opcodes(frames))
	}
	r := wire.NewReader(frames[teleport][1:])
	if id, x, y := r.ReadInt32(), int(r.ReadInt32()), int(r.ReadInt32()); id != objID || x < town.X-20 || x > town.X+20 || y < town.Y-20 || y > town.Y+20 {
		t.Fatalf("TeleportToLocation = %d at %d, %d, want %d within 20 of %d, %d", id, x, y, objID, town.X, town.Y)
	}
	if canceled := firstOpcode(frames, serverpackets.OpcodeMagicSkillCanceled); canceled < 0 || canceled > teleport {
		t.Fatalf("recall frames = %x, want the cast's MagicSkillCanceled ahead of the teleport", opcodes(frames))
	}
}
