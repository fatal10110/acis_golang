package ai

import (
	"testing"

	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

// TestFolkFollowStepGatesItsTarget pins the civilian NPC's follow step
// (NpcAI.thinkFollow), run from the periodic AI cycle: a follower 150 or
// more from its target takes up the run stance and, on a roll over 50 of
// every second cycle, walks toward a point near it; one nearer than 150
// holds still; a follow whose target plays dead (alike dead) is dropped by desire
// selection before it steps, whatever the distance.
func TestFolkFollowStepGatesItsTarget(t *testing.T) {
	t.Parallel()
	const cycles = 120 // a follower passes its 50 % roll every second cycle: missing all of them has a chance of 2^-60
	for _, tc := range []struct {
		name      string
		distance  int
		fakeDeath bool
		wantMoves bool
	}{
		{"far target is followed", 400, false, true},
		{"target exactly at the radius is followed", 150, false, true},
		{"target within the radius holds the follower", 149, false, false},
		{"far target playing dead is dropped", 400, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv, at := bootFolkDesires(t, nil)
			needsDrivenClock(t, srv)
			target := characterOf(t, srv)
			if tc.fakeDeath {
				obj, ok := srv.State.Player(srv.SoleObjectID(t))
				if !ok {
					t.Fatal("the character is not in the world")
				}
				victim, ok := network.OnlineCharacter(obj)
				if !ok {
					t.Fatalf("%T is not an online character", obj)
				}
				if !srv.PlayerQueue(t, victim.ObjectID()).Post(func() {
					e, err := effect.New(effect.Skill{ID: 1}, modelskill.EffectTemplate{Name: "FakeDeath"})
					if err != nil {
						t.Errorf("new FakeDeath effect: %v", err)
						return
					}
					e.Effected = victim
					victim.EffectList().Add(e)
				}) {
					t.Fatal("the character's queue is closed")
				}
				srv.Settle(t)
			}
			f := srv.SpawnFolkNPCAt(t, folkFighterTemplate(folkPatrolID), location.Location{X: at.X + tc.distance, Y: at.Y, Z: at.Z})
			srv.Settle(t)
			cycle(t, srv)
			if !f.Queue().Post(func() { f.AddFollowDesire(target, 100) }) {
				t.Fatal("the folk's queue is closed")
			}
			srv.Settle(t)

			moved := false
			for range cycles {
				cycle(t, srv)
				if f.IsMoving() {
					moved = true
					break
				}
			}
			if moved != tc.wantMoves {
				t.Fatalf("follower moved = %v, want %v", moved, tc.wantMoves)
			}
			switch {
			case tc.wantMoves:
				if !f.Running() {
					t.Fatal("the following NPC walks, want the run stance")
				}
			case tc.fakeDeath:
				if got := f.CurrentIntention(); got != actorai.IntentionIdle {
					t.Fatalf("intention after the dropped follow = %v, want idle", got)
				}
			}
		})
	}
}
