package ai

import (
	"strconv"
	"testing"
	"time"

	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference: npcs.properties MaxDriftRange (Config.MAX_DRIFT_RANGE, shipped
// 200) is the radius of a point spawn's territory (Spawn.isInMyTerritory, a
// 3D sphere), the range an Attackable strays before it heads home
// (Attackable.getDriftRange, a 2D circle) and the distance MonsterBehavior's
// 5001 timer makes a short-ranged monster flee by.

// driftRows are the drift ranges the scenarios run at: the shipped one, by
// no option, and one set through MaxDriftRange.
var driftRows = []struct {
	set, want int
}{{0, 200}, {350, 350}}

// bootDrift boots the behavior world with MaxDriftRange set to drift, or
// at its default for 0, beside a short-ranged warrior.
func bootDrift(t *testing.T, drift int) *warriorWorld {
	t.Helper()
	var extra []gameservertest.Option
	if drift != 0 {
		extra = append(extra, gameservertest.WithMaxDriftRange(drift))
	}
	return bootBehaviorsWith(t, extra, warriorTemplate(behaviorWarriorID, npc.AIParams{"AttackRange": "1"}))
}

// TestMaxDriftRangeBoundsTheSpawnTerritory pins the territory of an NPC
// spawned at a point: a sphere of the drift range around the spawn point,
// which a stand just inside keeps and one at the drift range leaves.
func TestMaxDriftRangeBoundsTheSpawnTerritory(t *testing.T) {
	t.Parallel()
	for _, row := range driftRows {
		t.Run(strconv.Itoa(row.want), func(t *testing.T) {
			t.Parallel()
			w := bootDrift(t, row.set)
			h := w.spawn(t, behaviorWarriorID, 50, 0)
			home := h.Instance.Home
			for _, tc := range []struct {
				dx   int
				want bool
			}{{row.want - 1, true}, {row.want, false}} {
				at := location.Location{X: home.X + tc.dx, Y: home.Y, Z: home.Z}
				if got := h.SpawnTerritoryContains(at); got != tc.want {
					t.Errorf("territory contains spawn point +%d = %v, want %v", tc.dx, got, tc.want)
				}
			}
		})
	}
}

// TestMaxDriftRangeLeashesTheMonsterHome pins the return home of a monster
// with no hate: within the drift range of its spawn point it stays; past
// it, it walks back to the spawn point.
func TestMaxDriftRangeLeashesTheMonsterHome(t *testing.T) {
	t.Parallel()
	for _, row := range driftRows {
		for _, tc := range []struct {
			name  string
			dx    int
			moves bool
		}{{"inside", row.want - 20, false}, {"outside", row.want + 20, true}} {
			t.Run(strconv.Itoa(row.want)+"/"+tc.name, func(t *testing.T) {
				t.Parallel()
				w := bootDrift(t, row.set)
				h := w.spawn(t, behaviorWarriorID, 50, 0)
				home := h.Instance.Home
				to := location.Location{X: home.X + tc.dx, Y: home.Y, Z: home.Z}
				w.onQueue(t, h, func() {
					script.NPCOf(h).InstantTeleportTo(script.Loc{X: to.X, Y: to.Y, Z: to.Z})
				})
				w.srv.ReadQueued(t, w.srv.Client)

				var returned bool
				w.onQueue(t, h, func() { returned = h.ReturnHome() })
				moves := framesOf(w.srv.ReadQueued(t, w.srv.Client), serverpackets.OpcodeMoveToLocation, h.ObjectID())
				if returned != tc.moves {
					t.Fatalf("return home %d from the spawn point = %v, want %v", tc.dx, returned, tc.moves)
				}
				if !tc.moves {
					if len(moves) != 0 {
						t.Fatalf("a monster within its drift range sent %d MoveToLocation frames, want none", len(moves))
					}
					return
				}
				if len(moves) != 1 {
					t.Fatalf("MoveToLocation frames = %d, want 1", len(moves))
				}
				if dest := moveDestination(moves[0]); dest.X != home.X || dest.Y != home.Y {
					t.Fatalf("return home destination = %v, want the spawn point %v", dest, home)
				}
			})
		}
	}
}

// TestMaxDriftRangeIsTheFleeDistance pins MonsterBehavior's 5001 timer: a
// short-ranged monster hit from up close wants, 10 s later, to run from its
// attacker by the drift range.
func TestMaxDriftRangeIsTheFleeDistance(t *testing.T) {
	t.Parallel()
	for _, row := range driftRows {
		t.Run(strconv.Itoa(row.want), func(t *testing.T) {
			t.Parallel()
			w := bootDrift(t, row.set)
			h := w.spawn(t, behaviorWarriorID, 50, 0)

			w.hit(t, h, 10)
			w.srv.Advance(t, 10*time.Second)
			d, ok := desire(h, actorai.IntentionFlee)
			if !ok || d.Distance != row.want {
				t.Fatalf("flee desire = %+v (%v), want one %d far", d, ok, row.want)
			}
		})
	}
}
