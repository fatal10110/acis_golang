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
		name          string
		geoFailures   int
		initialOpcode byte
		wantRecheck   bool
	}{
		{"ordinary walk", 9, serverpackets.OpcodeMoveToLocation, true},
		{"recovery teleport", 10, serverpackets.OpcodeValidateLocation, false},
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
			assertFrameOpcode(t, mustRead(t, c, "return home"), tc.initialOpcode, "return home")
			if !tc.wantRecheck {
				if x, y, z := hostile.Position(); (location.Location{X: x, Y: y, Z: z}) != home {
					t.Fatalf("position after recovery = (%d,%d,%d), want %+v", x, y, z, home)
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
