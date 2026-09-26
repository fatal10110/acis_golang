package combat

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

func TestReturnHomeRecoverySkipsWanderRecheck(t *testing.T) {
	for _, tc := range []struct {
		name        string
		geoFailures int
		wantRecheck bool
	}{
		{"ordinary walk", 9, true},
		{"recovery teleport", 10, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
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
			if got := hostile.AI().CurrentIntention(); got != ai.IntentionWander {
				t.Fatalf("CurrentIntention() = %v, want wander", got)
			}
			drainUntilQuiet(t, c)

			hostile.SetXYZ(home.X, home.Y+500, home.Z)
			for range tc.geoFailures {
				hostile.AddGeoPathFailCount()
			}
			if !hostile.ReturnHome() {
				t.Fatal("ReturnHome() = false outside drift range")
			}
			if tc.wantRecheck {
				assertFrameOpcode(t, mustRead(t, c, "return home"), serverpackets.OpcodeMoveToLocation, "return home")
			} else {
				// The first wander step is still walking, so the teleport's
				// abort stops it before the jump is announced.
				stop := mustRead(t, c, "StopMove")
				assertFrameOpcode(t, stop, serverpackets.OpcodeStopMove, "StopMove")
				if id := wireReader(stop[1:]).ReadInt32(); id != hostile.ObjectID() {
					t.Fatalf("StopMove object id = %d, want %d", id, hostile.ObjectID())
				}
				assertNPCTeleportFrames(t, c, mustRead(t, c, "TeleportToLocation"), hostile.ObjectID(), home)
				if hostile.IsMoving() {
					t.Fatal("IsMoving() = true after recovery teleport")
				}
				if x, y, z := hostile.Position(); (location.Location{X: x, Y: y, Z: z}) != home {
					t.Fatalf("position after recovery = (%d,%d,%d), want %+v", x, y, z, home)
				}
				if got := hostile.GeoPathFailCount(); got != 0 {
					t.Fatalf("GeoPathFailCount() after recovery = %d, want 0", got)
				}
			}

			// Even at the slowest 60-unit walk and largest random roll, the
			// delayed backwards step is due before five seconds.
			srv.Advance(t, 5*time.Second)
			frame := c.ReadWithTimeout(300 * time.Millisecond)
			if tc.wantRecheck {
				assertFrameOpcode(t, frame, serverpackets.OpcodeMoveToLocation, "wander recheck")
			} else if frame != nil {
				t.Fatalf("unexpected frame after recovery teleport: opcode %#x", frame[0])
			}
		})
	}
}

// The recovery teleport normally runs inside the AI think loop (idle wander
// step -> return home -> teleport), which holds the brain's lock while the
// teleport aborts the NPC's cast. The tick must finish and observers must see
// the teleport.
func TestReturnHomeRecoveryFromThinkLoop(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c := srv.Client
	startInWorld(t, c)

	home := location.Location{X: hostileX, Y: hostileY, Z: hostileZ}
	hostile := srv.SpawnMovingHostileNPCAt(t, "Monster", home, home)
	drainUntilQuiet(t, c)
	hostile.SetXYZ(home.X, home.Y+500, home.Z)
	for range 10 {
		hostile.AddGeoPathFailCount()
	}

	done := make(chan error, 1)
	go func() {
		for range 3 {
			if err := hostile.TickThink(); err != nil {
				done <- err
				return
			}
		}
		done <- nil
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("TickThink() error: %v", err)
		}
	case <-time.After(5 * time.Second):
		// A deadlocked tick would also block the harness cleanup, so fail
		// the binary with every goroutine's stack instead of t.Fatal.
		panic("think loop deadlocked on the recovery teleport")
	}

	// The wander step forces walk stance first; skip to the teleport.
	for {
		frame := c.ReadWithTimeout(time.Second)
		if frame == nil {
			t.Fatal("TeleportToLocation never arrived")
		}
		if frame[0] != serverpackets.OpcodeChangeMoveType {
			assertNPCTeleportFrames(t, c, frame, hostile.ObjectID(), home)
			break
		}
	}
	if x, y, z := hostile.Position(); (location.Location{X: x, Y: y, Z: z}) != home {
		t.Fatalf("position after recovery = (%d,%d,%d), want %+v", x, y, z, home)
	}
}

// A teleport that starts while another is still in progress is dropped: no
// frames, no movement, and the geo-path fail streak is left alone.
func TestNPCTeleportDroppedWhileAnotherInProgress(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c := srv.Client
	startInWorld(t, c)

	home := location.Location{X: hostileX, Y: hostileY, Z: hostileZ}
	hostile := srv.SpawnMovingHostileNPCAt(t, "Monster", home, home)
	drainUntilQuiet(t, c)
	for range 3 {
		hostile.AddGeoPathFailCount()
	}

	if !hostile.SetTeleporting(true) {
		t.Fatal("SetTeleporting(true) on an idle NPC reported no change")
	}
	hostile.TeleportTo(location.Location{X: home.X, Y: home.Y + 300, Z: home.Z})
	if frame := c.ReadWithTimeout(300 * time.Millisecond); frame != nil {
		t.Fatalf("dropped teleport sent opcode %#x", frame[0])
	}
	if x, y, z := hostile.Position(); (location.Location{X: x, Y: y, Z: z}) != home {
		t.Fatalf("position after dropped teleport = (%d,%d,%d), want %+v", x, y, z, home)
	}
	if got := hostile.GeoPathFailCount(); got != 3 {
		t.Fatalf("GeoPathFailCount() after dropped teleport = %d, want 3", got)
	}
	if !hostile.Teleporting() {
		t.Fatal("dropped teleport cleared the in-progress flag")
	}
}

// assertNPCTeleportFrames asserts what an observer who sees an NPC before and
// after its teleport receives, starting from the already-read frame:
// TeleportToLocation to the destination with the fast-teleport flag off,
// DeleteObject as the NPC leaves the grid, then NpcInfo as it re-enters.
func assertNPCTeleportFrames(t *testing.T, c *scriptedClient, frame []byte, objectID int32, to location.Location) {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeTeleportToLocation, "TeleportToLocation")
	if len(frame) != 21 {
		t.Fatalf("TeleportToLocation length = %d, want 21", len(frame))
	}
	r := wireReader(frame[1:])
	for _, field := range []struct {
		name string
		want int32
	}{
		{"object id", objectID},
		{"x", int32(to.X)},
		{"y", int32(to.Y)},
		{"z", int32(to.Z)},
		{"fast teleport", 0},
	} {
		if got := r.ReadInt32(); got != field.want {
			t.Fatalf("TeleportToLocation %s = %d, want %d", field.name, got, field.want)
		}
	}

	frame = mustRead(t, c, "DeleteObject")
	assertFrameOpcode(t, frame, serverpackets.OpcodeDeleteObject, "DeleteObject")
	if len(frame) != 9 {
		t.Fatalf("DeleteObject length = %d, want 9", len(frame))
	}
	r = wireReader(frame[1:])
	if id := r.ReadInt32(); id != objectID {
		t.Fatalf("DeleteObject object id = %d, want %d", id, objectID)
	}
	if mode := r.ReadInt32(); mode != 1 {
		t.Fatalf("DeleteObject mode = %d, want 1 (delete without standing up)", mode)
	}

	frame = mustRead(t, c, "NpcInfo")
	assertFrameOpcode(t, frame, serverpackets.OpcodeNPCInfo, "NpcInfo")
	if id := wireReader(frame[1:]).ReadInt32(); id != objectID {
		t.Fatalf("NpcInfo object id = %d, want %d", id, objectID)
	}
}
