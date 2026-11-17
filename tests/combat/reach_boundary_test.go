package combat

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attack"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestMeleeReachLeavesExactIntegerBoundaryOut pins the melee reach to the
// strict 2D radius check on the square-root distance (Point2D.isIn2DRadius,
// Point2D.java:146): a rooted monster whose reach is exactly 101 units
// refuses to swing at a player standing at offset (20, 99), exactly 101
// away, and swings at one at (20, 98).
func TestMeleeReachLeavesExactIntegerBoundaryOut(t *testing.T) {
	t.Parallel()
	const reach = 101
	for _, tc := range []struct {
		name   string
		dy     int
		inside bool
	}{{name: "on the boundary", dy: 99}, {name: "one unit inside", dy: 98, inside: true}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := gameservertest.Boot(t,
				gameservertest.WithCharacter("Newbie", 5, 0),
				gameservertest.WithWantChars(1),
			)
			c := srv.Client
			startInWorld(t, c)
			player := intakePlayerOf(t, srv, srv.SoleObjectID(t))
			// The monster's template cannot move, so its swing gate measures
			// the reach where it stands: weapon range, both radii and the
			// 10-unit grace for a standing target, filling exactly 101.
			tmpl := gameservertest.AttackingHostileTemplate()
			tmpl.BaseAttackRange = 40
			tmpl.CollisionRadius = float64(reach-10-tmpl.BaseAttackRange) - player.CollisionRadius()
			hostile := srv.SpawnAttackingHostileNPCTemplate(t, tmpl, location.Location{X: playerOrigin.X - 20, Y: playerOrigin.Y - tc.dy, Z: playerOrigin.Z})
			drainUntilQuiet(t, c)
			if !hostile.MovementDisabled() {
				t.Fatal("monster can move, want its swing gate to measure the reach in place")
			}
			if got := attack.PhysicalReach(hostile.PhysicalAttackRange(), hostile.CollisionRadius(), player.CollisionRadius(), false); got != reach {
				t.Fatalf("monster reach = %d, want %d", got, reach)
			}
			if got := hostile.CanAttack(player); got != tc.inside {
				t.Fatalf("monster may swing at the player %d units off = %v, want %v", tc.dy, got, tc.inside)
			}
		})
	}
}
