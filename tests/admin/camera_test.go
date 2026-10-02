package admin

import (
	"bytes"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

func encodeValidatePosition(x, y, z int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeValidatePosition)
	w.WriteInt32(x)
	w.WriteInt32(y)
	w.WriteInt32(z)
	w.WriteInt32(0) // heading
	w.WriteInt32(0) // boat
	return w.Bytes()
}

// cameraModeAt returns the index of the CameraMode frame in frames carrying
// mode, failing unless there is exactly one CameraMode.
func cameraModeAt(t *testing.T, frames [][]byte, mode byte) int {
	t.Helper()
	at := -1
	for i, f := range frames {
		if f[0] != serverpackets.OpcodeCameraMode {
			continue
		}
		if at >= 0 {
			t.Fatalf("frames = %x, want one CameraMode", testsupport.FrameOpcodes(frames))
		}
		if want := []byte{serverpackets.OpcodeCameraMode, mode, 0, 0, 0}; !bytes.Equal(f, want) {
			t.Fatalf("CameraMode = % x, want % x", f, want)
		}
		at = i
	}
	if at < 0 {
		t.Fatalf("frames = %x, want a CameraMode", testsupport.FrameOpcodes(frames))
	}
	return at
}

// teleportAt returns the index of objectID's TeleportToLocation in frames
// and its destination.
func teleportAt(t *testing.T, frames [][]byte, objectID int32) (int, [3]int32) {
	t.Helper()
	for i, f := range frames {
		if f[0] != serverpackets.OpcodeTeleportToLocation {
			continue
		}
		if id, at := teleportTo(t, f); id == objectID {
			return i, at
		}
	}
	t.Fatalf("frames = %x, want a TeleportToLocation of %d", testsupport.FrameOpcodes(frames), objectID)
	return -1, [3]int32{}
}

// TestAdminCamera pins //camera (AdminAdmin.java admin_camera) and its
// ValidatePosition branch (ValidatePosition.java:43-51):
//
//   - turning it on sends CameraMode(1), hides the GM and teleports it
//     where it stands; turning it off sends CameraMode(0), shows the GM and
//     teleports it where it stands;
//   - while it is on, the position the GM's client reports is taken as is:
//     no fall damage, no ValidateLocation, however far it is; a report
//     outside the world changes nothing;
//   - once it is off, a far report is corrected again.
func TestAdminCamera(t *testing.T) {
	t.Parallel()
	srv, gmID := bootAdmin(t, adminLevel, gameservertest.WithFallingDamage(true))
	gm := srv.Client
	enterWorld(t, gm)
	watcher, watcherID := addPlayer(t, srv, "player2", "Watcher", userLevel)
	drain(t, gm)
	drain(t, watcher)
	gmChar := onlineCharacter(t, srv, gmID)
	watcherChar := onlineCharacter(t, srv, watcherID)

	// The control: without the camera a far report is corrected and the
	// server position stays.
	frames := exchange(t, gm, encodeValidatePosition(spawnX+3000, spawnY, spawnZ))
	if len(objectFrames(frames, serverpackets.OpcodeValidateLocation, gmID)) != 1 {
		t.Fatalf("far report frames = %x, want one ValidateLocation", testsupport.FrameOpcodes(frames))
	}
	if x, _, _ := srv.PlayerPosition(t, gmID); x != spawnX {
		t.Fatalf("far report moved the GM to X %d", x)
	}

	frames = exchange(t, gm, encodeBuildCmd("camera"))
	mode := cameraModeAt(t, frames, 1)
	tele, at := teleportAt(t, frames, gmID)
	if mode > tele {
		t.Fatalf("frames = %x, want CameraMode before the teleport", testsupport.FrameOpcodes(frames))
	}
	if at != [3]int32{spawnX, spawnY, spawnZ} {
		t.Fatalf("//camera teleport = %v, want where the GM stands", at)
	}
	if !gmChar.Invisible() {
		t.Fatal("//camera left the GM visible")
	}
	appear(t, gm)
	settle(t, watcher)
	if watcherChar.Knows(gmChar) {
		t.Fatal("a player knows the GM under the camera")
	}

	// A fall far beyond the safe height and a far report are both taken
	// as is. The far one leaves the watcher's surroundings, so the GM
	// forgets it: the world follows the reported position.
	hp := gmChar.HP()
	for _, report := range []struct {
		at      [3]int32
		forgets bool
	}{
		{[3]int32{spawnX, spawnY, spawnZ - 2000}, false},
		{[3]int32{spawnX + 9000, spawnY + 500, spawnZ - 2000}, true},
	} {
		frames = exchange(t, gm, encodeValidatePosition(report.at[0], report.at[1], report.at[2]))
		want := 0
		if report.forgets {
			want = 1
			if len(objectFrames(frames, serverpackets.OpcodeDeleteObject, watcherID)) != 1 {
				t.Fatalf("camera report %v frames = %x, want the watcher forgotten", report.at, testsupport.FrameOpcodes(frames))
			}
		}
		if len(frames) != want {
			t.Fatalf("camera report %v frames = %x, want no correction and no fall damage", report.at, testsupport.FrameOpcodes(frames))
		}
		if x, y, z := srv.PlayerPosition(t, gmID); x != int(report.at[0]) || y != int(report.at[1]) || z != int(report.at[2]) {
			t.Fatalf("camera report %v: GM at %d %d %d, want the report", report.at, x, y, z)
		}
	}
	if gmChar.HP() != hp {
		t.Fatalf("GM HP = %v after a camera fall, want %v", gmChar.HP(), hp)
	}

	// Outside the world nothing changes.
	exchange(t, gm, encodeValidatePosition(world.MaxX+100, spawnY, spawnZ))
	if x, _, _ := srv.PlayerPosition(t, gmID); x != spawnX+9000 {
		t.Fatalf("out-of-world report moved the GM to X %d", x)
	}

	frames = exchange(t, gm, encodeBuildCmd("camera"))
	mode = cameraModeAt(t, frames, 0)
	tele, at = teleportAt(t, frames, gmID)
	if mode > tele {
		t.Fatalf("frames = %x, want CameraMode before the teleport", testsupport.FrameOpcodes(frames))
	}
	if at != [3]int32{spawnX + 9000, spawnY + 500, spawnZ - 2000} {
		t.Fatalf("//camera off teleport = %v, want where the camera left the GM", at)
	}
	if gmChar.Invisible() {
		t.Fatal("a second //camera left the GM invisible")
	}
	appear(t, gm)

	// Off again, a far report is corrected.
	frames = exchange(t, gm, encodeValidatePosition(spawnX, spawnY, spawnZ-2000))
	if len(objectFrames(frames, serverpackets.OpcodeValidateLocation, gmID)) != 1 {
		t.Fatalf("far report after the camera frames = %x, want one ValidateLocation", testsupport.FrameOpcodes(frames))
	}
	if x, _, _ := srv.PlayerPosition(t, gmID); x != spawnX+9000 {
		t.Fatalf("far report after the camera moved the GM to X %d", x)
	}
}
