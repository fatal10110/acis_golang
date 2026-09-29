package combat

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// sightGeo is passable movement geo whose line-of-sight query answers see.
type sightGeo struct {
	gameservertest.Geo
	see bool
}

func (g sightGeo) CanSeeActor(int, int, int, float64, int, int, int, float64) bool { return g.see }

// npcActivity lets d pass in 100ms steps and reports whether the client saw
// id move (MoveToLocation or MoveToPawn) or swing (Attack).
func npcActivity(t *testing.T, srv *gameservertest.Server, c *scriptedClient, id int32, d time.Duration) (moved, attacked bool) {
	t.Helper()
	for passed := time.Duration(0); passed < d; passed += 100 * time.Millisecond {
		srv.Advance(t, 100*time.Millisecond)
		for frame := c.ReadWithTimeout(20 * time.Millisecond); frame != nil; frame = c.ReadWithTimeout(20 * time.Millisecond) {
			switch frame[0] {
			case serverpackets.OpcodeMoveToLocation, serverpackets.OpcodeMoveToPawn:
				if wireReader(frame[1:]).ReadInt32() == id {
					moved = true
				}
			case serverpackets.OpcodeAttack:
				if wireReader(frame[1:]).ReadInt32() == id {
					attacked = true
				}
			}
		}
	}
	return moved, attacked
}

// TestHalishaChestNeverChasesOutOfRangePlayer pins the Halisha chest's
// movement lock: its template allows movement, yet with hate on a player out
// of reach it reports movement disabled and never walks, while a Monster on
// the same template chases.
func TestHalishaChestNeverChasesOutOfRangePlayer(t *testing.T) {
	for _, tt := range []struct {
		kind  string
		chase bool
	}{
		{"HalishaChest", false},
		{"Monster", true},
	} {
		t.Run(tt.kind, func(t *testing.T) {
			t.Parallel()
			srv := gameservertest.Boot(t,
				gameservertest.WithCharacter("Newbie", 5, 0),
				gameservertest.WithWantChars(1),
			)
			c := srv.Client
			startInWorld(t, c)
			player := liveCombatant(t, srv)

			tmpl := gameservertest.MovingHostileTemplate(tt.kind)
			if !tmpl.CanMove {
				t.Fatal("fixture template must allow movement")
			}
			px, py, pz := player.Position()
			home := location.Location{X: px + 600, Y: py, Z: pz}
			chest := srv.SpawnMovingHostileNPCTemplate(t, tmpl, home, home)
			drainUntilQuiet(t, c)

			if got := chest.MovementDisabled(); got == tt.chase {
				t.Fatalf("MovementDisabled() = %v, want %v", got, !tt.chase)
			}
			chest.AddCombatDamageHate(player, 50)
			if got := chest.AI().CurrentIntention(); got != ai.IntentionAttack {
				t.Fatalf("CurrentIntention() = %v, want %v", got, ai.IntentionAttack)
			}

			moved, _ := npcActivity(t, srv, c, chest.ObjectID(), time.Second)
			if moved != tt.chase {
				t.Fatalf("%s moved = %v, want %v", tt.kind, moved, tt.chase)
			}
			if !tt.chase {
				if x, y, z := chest.Position(); (location.Location{X: x, Y: y, Z: z}) != home {
					t.Fatalf("Halisha chest position = (%d,%d,%d), want %+v", x, y, z, home)
				}
			}
		})
	}
}

// TestNPCClosesInOnUnseenTargetInReach pins the offensive follow's
// line-of-sight branch: a monster with hate on a player inside its reach
// swings in place when it can see the player, walks toward the player
// instead of swinging when terrain blocks its sight, and, rooted behind that
// terrain, neither walks nor swings.
func TestNPCClosesInOnUnseenTargetInReach(t *testing.T) {
	for _, tt := range []struct {
		name   string
		see    bool
		rooted bool
		move   bool
		attack bool
	}{
		{name: "in sight", see: true, attack: true},
		{name: "out of sight", see: false, move: true},
		{name: "rooted out of sight", see: false, rooted: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv := gameservertest.Boot(t,
				gameservertest.WithCharacter("Newbie", 5, 0),
				gameservertest.WithWantChars(1),
			)
			c := srv.Client
			startInWorld(t, c)
			player := liveCombatant(t, srv)

			px, py, pz := player.Position()
			home := location.Location{X: px + 45, Y: py, Z: pz}
			monster := srv.SpawnMovingHostileNPCAtGeo(t, "Monster", home, home, sightGeo{see: tt.see})
			monster.Instance.Template.BaseAttackRange = 40
			assertInReach(t, monster, player.CollisionRadius(), home, location.Location{X: px, Y: py, Z: pz})
			drainUntilQuiet(t, c)
			if tt.rooted {
				landEffect(t, monster, "Root")
				drainUntilQuiet(t, c)
			}

			monster.AddCombatDamageHate(player, 50)

			moved, attacked := npcActivity(t, srv, c, monster.ObjectID(), 2*time.Second)
			if moved != tt.move || attacked != tt.attack {
				t.Fatalf("moved, attacked = %v, %v; want %v, %v", moved, attacked, tt.move, tt.attack)
			}
		})
	}
}

// TestHoldAttackDesireNeverChasesOutOfRange pins the move-to-target flag: a
// monster whose attack intention holds its ground stays put while the
// target is out of reach, where a moving attack chases.
func TestHoldAttackDesireNeverChasesOutOfRange(t *testing.T) {
	for _, hold := range []bool{false, true} {
		name := "moving"
		if hold {
			name = "hold"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			srv := gameservertest.Boot(t,
				gameservertest.WithCharacter("Newbie", 5, 0),
				gameservertest.WithWantChars(1),
			)
			c := srv.Client
			startInWorld(t, c)
			player := liveCombatant(t, srv)

			px, py, pz := player.Position()
			home := location.Location{X: px + 600, Y: py, Z: pz}
			monster := srv.SpawnMovingHostileNPCAt(t, "Monster", home, home)
			drainUntilQuiet(t, c)

			if hold {
				monster.AddAttackDesireHold(player, 50)
			} else {
				monster.AddAttackDesire(player, 50)
			}
			if got := monster.AI().CurrentIntention(); got != ai.IntentionAttack {
				t.Fatalf("CurrentIntention() = %v, want %v", got, ai.IntentionAttack)
			}

			moved, _ := npcActivity(t, srv, c, monster.ObjectID(), time.Second)
			if moved == hold {
				t.Fatalf("moved = %v, want %v", moved, !hold)
			}
		})
	}
}

// assertInReach guards the fixture geometry: the monster at at must have
// the player at target inside its attack reach but outside the tighter
// footprint-only follow radius, so the unseen case really has to walk.
func assertInReach(t *testing.T, monster *npc.Hostile, targetRadius float64, at, target location.Location) {
	t.Helper()
	reach := int(float64(monster.PhysicalAttackRange()) + monster.CollisionRadius() + targetRadius)
	footprint := int(targetRadius + monster.CollisionRadius() + targetRadius)
	if !at.In2DRadius(target, reach) || at.In2DRadius(target, footprint) {
		t.Fatalf("fixture distance: reach %d, footprint %d, positions %+v -> %+v", reach, footprint, at, target)
	}
}
