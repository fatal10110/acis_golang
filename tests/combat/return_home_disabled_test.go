package combat

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

func TestReturnHomeMovementDisabledSkipsWalkButKeepsRecoveryTeleport(t *testing.T) {
	for _, lock := range []string{"root", "canMove=false"} {
		t.Run(lock, func(t *testing.T) {
			t.Parallel()
			srv := gameservertest.Boot(t,
				gameservertest.WithCharacter("Newbie", 5, 0),
				gameservertest.WithWantChars(1),
			)
			c := srv.Client
			startInWorld(t, c)

			home := location.Location{X: hostileX, Y: hostileY, Z: hostileZ}
			tmpl := gameservertest.MovingHostileTemplate("Monster")
			if lock == "canMove=false" {
				tmpl.CanMove = false
			}
			hostile := srv.SpawnMovingHostileNPCTemplate(t, tmpl, home, home)
			drainUntilQuiet(t, c)
			if lock == "root" {
				landEffect(t, hostile, "Root")
				drainUntilQuiet(t, c)
			}
			stranded := location.Location{X: home.X, Y: home.Y + 500, Z: home.Z}
			hostile.SetXYZ(stranded.X, stranded.Y, stranded.Z)
			if !hostile.MovementDisabled() || !hostile.ReturnHome() {
				t.Fatal("movement-disabled ReturnHome should return true outside drift range")
			}
			assertChangeMoveType(t, mustRead(t, c, "ChangeMoveType"), hostile.ObjectID(), false)
			if hostile.Move().Moving() {
				t.Fatal("movement-disabled NPC started walking home")
			}
			if frame := c.ReadWithTimeout(300 * time.Millisecond); frame != nil {
				t.Fatalf("unexpected packet after disabled ReturnHome: opcode %#x", frame[0])
			}

			for range move.HomeGeoFailLimit {
				hostile.AddGeoPathFailCount()
			}
			if !hostile.ReturnHome() {
				t.Fatal("ReturnHome() = false at geo fail limit, want teleport")
			}
			assertFrameOpcode(t, mustRead(t, c, "ValidateLocation"), serverpackets.OpcodeValidateLocation, "ValidateLocation")
			if x, y, z := hostile.Position(); (location.Location{X: x, Y: y, Z: z}) != home {
				t.Fatalf("position after recovery = (%d,%d,%d), want %+v", x, y, z, home)
			}
			if hostile.Move().Moving() {
				t.Fatal("recovery teleport started a walk")
			}
		})
	}
}
