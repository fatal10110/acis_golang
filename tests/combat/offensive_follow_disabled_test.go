package combat

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestRootedNPCDoesNotChaseOutOfRangeTarget gives a monster hate on a target
// well outside its reach. Free to move, it chases; rooted, it keeps the attack
// intention but starts no chase, and stays where it is.
func TestRootedNPCDoesNotChaseOutOfRangeTarget(t *testing.T) {
	for _, rooted := range []bool{false, true} {
		name := "free"
		if rooted {
			name = "rooted"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			srv := gameservertest.Boot(t,
				gameservertest.WithCharacter("Newbie", 5, 0),
				gameservertest.WithWantChars(1),
			)
			c := srv.Client
			startInWorld(t, c)

			home := location.Location{X: hostileX, Y: hostileY, Z: hostileZ}
			chaser := srv.SpawnMovingHostileNPCAt(t, "Monster", home, home)
			target := srv.SpawnHostileNPCAt(t, location.Location{X: home.X + 600, Y: home.Y, Z: home.Z})
			drainUntilQuiet(t, c)
			if rooted {
				landEffect(t, chaser, "Root")
				drainUntilQuiet(t, c)
			}

			chaser.AddCombatDamageHate(target, 50)

			if got := chaser.AI().CurrentIntention(); got != ai.IntentionAttack {
				t.Fatalf("CurrentIntention() = %v, want %v", got, ai.IntentionAttack)
			}
			moving := chaser.Move().Moving()
			if rooted && moving {
				t.Fatal("rooted monster started chasing an out-of-range target")
			}
			if !rooted && !moving {
				t.Fatal("free monster did not chase an out-of-range target")
			}
			if rooted {
				if x, y, z := chaser.Position(); (location.Location{X: x, Y: y, Z: z}) != home {
					t.Fatalf("rooted monster position = (%d,%d,%d), want %+v", x, y, z, home)
				}
			}
		})
	}
}
