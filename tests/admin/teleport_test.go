package admin

import (
	"bytes"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/restart"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// teleportOf returns the destination of objectID's TeleportToLocation among
// frames, failing when there is none.
func teleportOf(t *testing.T, frames [][]byte, objectID int32) [3]int32 {
	t.Helper()
	for _, frame := range frames {
		if frame[0] != serverpackets.OpcodeTeleportToLocation {
			continue
		}
		if id, at := teleportTo(t, frame); id == objectID {
			return at
		}
	}
	t.Fatalf("frames = %x, want a TeleportToLocation of %d", testsupport.FrameOpcodes(frames), objectID)
	return [3]int32{}
}

// noTeleport fails when frames hold any TeleportToLocation.
func noTeleport(t *testing.T, frames [][]byte) {
	t.Helper()
	for _, frame := range frames {
		if frame[0] == serverpackets.OpcodeTeleportToLocation {
			t.Fatalf("frames = %x, want no TeleportToLocation", testsupport.FrameOpcodes(frames))
		}
	}
}

// readTeleport reads c's frames until objectID's TeleportToLocation and
// returns its destination.
func readTeleport(t *testing.T, c *testsupport.ScriptedClient, objectID int32) [3]int32 {
	t.Helper()
	for range 100 {
		frame := c.Read()
		if frame[0] != serverpackets.OpcodeTeleportToLocation {
			continue
		}
		if id, at := teleportTo(t, frame); id == objectID {
			return at
		}
	}
	t.Fatalf("no TeleportToLocation of %d within 100 frames", objectID)
	return [3]int32{}
}

// appear completes a teleport the way the client does once it has loaded
// the destination.
func appear(t *testing.T, c *testsupport.ScriptedClient) {
	t.Helper()
	c.Send(wire.NewPacketWriter(clientpackets.OpcodeAppearing).Bytes())
	drain(t, c)
}

// TestAdminTeleport pins AdminTeleport.java's //teleport, //teleportto,
// //recall and //tele: the GM jumps to coordinates or to a player, a player
// is brought to the GM (who loses the selection), coordinates that do not
// parse open the teleport panel, and an unknown name is an invalid target.
func TestAdminTeleport(t *testing.T) {
	t.Parallel()
	srv, gmID := bootAdmin(t, adminLevel)
	gm := srv.Client
	enterWorld(t, gm)
	other, otherID := addPlayer(t, srv, "player2", "Player", userLevel)
	drain(t, gm)

	assertPage(t, exchange(t, gm, encodeBuildCmd("tele")), "<title>Teleport</title>")
	assertPage(t, exchange(t, gm, encodeBuildCmd("teleport")), "<title>Teleport</title>")
	assertPage(t, exchange(t, gm, encodeBuildCmd("teleport 1000")), "<title>Teleport</title>")
	assertPage(t, exchange(t, gm, encodeBuildCmd("teleport 1000 y")), "<title>Teleport</title>")
	assertPage(t, exchange(t, gm, encodeBuildCmd("teleport 1000 2000 z")), "<title>Teleport</title>")

	if at := teleportOf(t, exchange(t, gm, encodeBuildCmd("teleport 1000 2000 300")), gmID); at != [3]int32{1000, 2000, 300} {
		t.Fatalf("//teleport destination = %v, want 1000 2000 300", at)
	}
	appear(t, gm)
	if x, y, z := srv.PlayerPosition(t, gmID); x != 1000 || y != 2000 || z != 300 {
		t.Fatalf("GM position = %d %d %d, want 1000 2000 300", x, y, z)
	}
	drain(t, other)

	// Without Z the GM keeps its own height (no geodata loaded here).
	if at := teleportOf(t, exchange(t, gm, encodeBuildCmd("teleport 1100 2100")), gmID); at != [3]int32{1100, 2100, 300} {
		t.Fatalf("//teleport X Y destination = %v, want 1100 2100 300", at)
	}
	appear(t, gm)
	drain(t, other)

	for _, line := range []string{"teleportto", "teleportto Nobody", "recall", "recall Nobody", "recall party", "recall clan Nobody"} {
		frames := exchange(t, gm, encodeBuildCmd(line))
		if len(frames) != 1 {
			t.Fatalf("//%s frames = %x, want INVALID_TARGET", line, testsupport.FrameOpcodes(frames))
		}
		assertStatic(t, frames[0], serverpackets.SystemMessageInvalidTarget)
	}

	// //recall brings the player to the GM; the player's name matches
	// whatever its case.
	frames := exchange(t, gm, encodeBuildCmd("recall player"))
	if !bytes.Contains(testsupport.FrameOpcodes(frames), []byte{serverpackets.OpcodeActionFailed}) {
		t.Fatalf("//recall GM frames = %x, want the selection drop's ActionFailed", testsupport.FrameOpcodes(frames))
	}
	if at := readTeleport(t, other, otherID); at != [3]int32{1100, 2100, 300} {
		t.Fatalf("//recall destination = %v, want the GM's 1100 2100 300", at)
	}
	appear(t, other)
	drain(t, gm)
	if x, y, _ := srv.PlayerPosition(t, otherID); x != 1100 || y != 2100 {
		t.Fatalf("recalled position = %d %d, want 1100 2100", x, y)
	}

	// //recall clan of a player in no clan brings that player alone.
	exchange(t, gm, encodeBuildCmd("teleport 500 600 300"))
	appear(t, gm)
	drain(t, other)
	exchange(t, gm, encodeBuildCmd("recall clan Player"))
	if at := readTeleport(t, other, otherID); at != [3]int32{500, 600, 300} {
		t.Fatalf("//recall clan destination = %v, want 500 600 300", at)
	}
	appear(t, other)
	drain(t, gm)

	// //teleportto takes the GM to the player.
	exchange(t, gm, encodeBuildCmd("teleport 9000 9000 300"))
	appear(t, gm)
	drain(t, other)
	if at := teleportOf(t, exchange(t, gm, encodeBuildCmd("teleportto Player")), gmID); at != [3]int32{500, 600, 300} {
		t.Fatalf("//teleportto destination = %v, want the player's 500 600 300", at)
	}
}

// TestAdminInstantMove pins //instant_move (AdminTeleport.java:38-58,
// MoveBackwardToLocation.java:95-103): mode 1 teleports the next move
// click and then walks again, mode 2 teleports every click, mode 0 walks,
// and a mode outside 0-2 is refused with its usage.
func TestAdminInstantMove(t *testing.T) {
	t.Parallel()
	srv, gmID := bootAdmin(t, adminLevel)
	gm := srv.Client
	enterWorld(t, gm)

	for _, line := range []string{"instant_move 3", "instant_move -1", "instant_move x"} {
		assertTexts(t, exchange(t, gm, encodeBuildCmd(line)), "Usage: //instant_move [0|1|2]")
	}
	// A move click walks.
	noTeleport(t, exchange(t, gm, encodeMove(spawnX+50, spawnY, spawnZ)))
	drain(t, gm)

	assertTexts(t, exchange(t, gm, encodeBuildCmd("instant_move")))
	frames := exchange(t, gm, encodeMove(2000, 3000, 100))
	if frames[0][0] != serverpackets.OpcodeActionFailed {
		t.Fatalf("teleport-mode move frames = %x, want ActionFailed first", testsupport.FrameOpcodes(frames))
	}
	// The click's floor height is raised to head height, and with no
	// geodata it stays there.
	if at := teleportOf(t, frames, gmID); at[0] != 2000 || at[1] != 3000 {
		t.Fatalf("teleport-mode destination = %v, want 2000 3000", at)
	}
	appear(t, gm)
	// Mode 1 was spent: the next click walks.
	noTeleport(t, exchange(t, gm, encodeMove(2050, 3000, 100)))
	drain(t, gm)

	assertTexts(t, exchange(t, gm, encodeBuildCmd("instant_move 2")))
	for i, x := range []int32{4000, 5000} {
		if at := teleportOf(t, exchange(t, gm, encodeMove(x, 3000, 100)), gmID); at[0] != x {
			t.Fatalf("mode 2 click %d destination = %v, want X %d", i, at, x)
		}
		appear(t, gm)
	}
	// Far beyond the 9900 walk limit, a teleport-mode click still jumps.
	if at := teleportOf(t, exchange(t, gm, encodeMove(-50000, 3000, 100)), gmID); at[0] != -50000 {
		t.Fatalf("mode 2 far click destination = %v, want X -50000", at)
	}
	appear(t, gm)

	assertTexts(t, exchange(t, gm, encodeBuildCmd("instant_move 0")))
	noTeleport(t, exchange(t, gm, encodeMove(-50050, 3000, 100)))
}

// TestAdminSendHome pins //sendhome (AdminTeleport.java:139-158): the named
// player, else the selected one, else the GM goes to its nearest town.
func TestAdminSendHome(t *testing.T) {
	t.Parallel()
	town := location.Location{X: 20000, Y: 20000, Z: 300}
	table := &restart.Table{Points: []restart.Point{{
		Name:       "TestTown",
		Points:     []location.Location{town},
		ChaoPoints: []location.Location{town},
		MapRegions: []location.Point{{
			X: (spawnX-world.MinX)/world.TileSize + world.TileXMin,
			Y: (spawnY-world.MinY)/world.TileSize + world.TileYMin,
		}},
	}}}
	srv, gmID := bootAdmin(t, adminLevel, gameservertest.WithRestartPoints(table))
	gm := srv.Client
	enterWorld(t, gm)
	other, otherID := addPlayer(t, srv, "player2", "Player", userLevel)
	drain(t, gm)

	frames := exchange(t, gm, encodeBuildCmd("sendhome Nobody"))
	if len(frames) != 1 {
		t.Fatalf("//sendhome Nobody frames = %x, want INVALID_TARGET", testsupport.FrameOpcodes(frames))
	}
	assertStatic(t, frames[0], serverpackets.SystemMessageInvalidTarget)

	exchange(t, gm, encodeBuildCmd("sendhome Player"))
	if at := readTeleport(t, other, otherID); !nearTown(at, town) {
		t.Fatalf("//sendhome Player destination = %v, want within 20 of %v", at, town)
	}
	appear(t, other)
	drain(t, gm)

	if at := teleportOf(t, exchange(t, gm, encodeBuildCmd("sendhome")), gmID); !nearTown(at, town) {
		t.Fatalf("//sendhome destination = %v, want within 20 of %v", at, town)
	}
}

func nearTown(at [3]int32, town location.Location) bool {
	dx, dy := int(at[0])-town.X, int(at[1])-town.Y
	return dx >= -20 && dx <= 20 && dy >= -20 && dy <= 20
}
