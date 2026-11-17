package combat

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestLooseFollowStepHoldsInsideAndStopsAtItsRadius pins the follow step of
// an NPC with no master (NpcAI.thinkFollow): it acts every second think
// cycle, picks a spot around the target only while the target is not
// strictly inside 150 units on the plane (isIn2DRadius is a strict test, so
// exactly 150 moves). A target playing dead is alike dead: desire selection
// prunes the follow, so the follower never takes a step toward it.
func TestLooseFollowStepHoldsInsideAndStopsAtItsRadius(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		distance  int
		fakeDeath bool
		wantMoves bool
	}{
		{"just inside the radius stays", 149, false, false},
		{"exactly at the radius moves", 150, false, true},
		{"far target moves", 400, false, true},
		{"far target playing dead is dropped", 400, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := gameservertest.Boot(t,
				gameservertest.WithCharacter("Newbie", 5, 0),
				gameservertest.WithWantChars(1),
			)
			startInWorld(t, srv.Client)
			objID := srv.SoleObjectID(t)
			victim := livePlayer(t, srv, objID)
			px, py, pz := srv.PlayerPosition(t, objID)
			if tc.fakeDeath {
				lieInFakeDeath(t, srv, onlineVictim(t, srv, objID), false)
			}
			tmpl := gameservertest.MovingHostileTemplate("Monster")
			tmpl.NoSleepMode = true
			at := location.Location{X: px + tc.distance, Y: py, Z: pz}
			hostile := srv.SpawnMovingHostileNPCTemplate(t, tmpl, at, at)
			drainUntilQuiet(t, srv.Client)
			if err := hostile.TickThink(); err != nil {
				t.Fatalf("spawn-cycle TickThink() error: %v", err)
			}
			// Every roll is the top of its range: the step passes its
			// 50 % roll and picks a spot 300 units out at angle 2 pi.
			hostile.SetRollSource(func(n int) int { return n - 1 })

			hostile.AI().AddFollowDesire(victim, 100)
			// The follow step acts on the second of the AI clock's three
			// steps.
			for range 3 {
				hostile.Tick()
				if err := hostile.TickThink(); err != nil {
					t.Fatalf("TickThink() error: %v", err)
				}
			}
			wantIntention := ai.IntentionFollow
			if tc.fakeDeath {
				wantIntention = ai.IntentionIdle
			}
			if got := hostile.AI().CurrentIntention(); got != wantIntention {
				t.Fatalf("CurrentIntention() = %v, want %v", got, wantIntention)
			}
			follow := &ai.Desire{Kind: ai.IntentionFollow, FinalTarget: victim}
			if held := hostile.AI().Desires().Has(follow); held == tc.fakeDeath {
				t.Fatalf("follow desire queued = %v, want %v", held, !tc.fakeDeath)
			}
			if got := hostile.IsMoving(); got != tc.wantMoves {
				t.Fatalf("hostile moving after its follow steps = %v, want %v", got, tc.wantMoves)
			}
		})
	}
}
