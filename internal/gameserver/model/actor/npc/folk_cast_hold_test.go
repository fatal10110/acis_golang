package npc

import (
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// TestFolkCastDesireHoldNeedsReach pins NpcAI.addCastDesire's hold gate on
// a civilian NPC: a cast that must not walk is queued only when its target
// stands strictly within the skill's range plus both bodies, measured flat;
// a cast that may walk is queued at any distance, and skipping the
// conditions skips the reuse and cost gate.
func TestFolkCastDesireHoldNeedsReach(t *testing.T) {
	for _, tc := range []struct {
		name               string
		dist               int
		check, move, canDo bool
		want               int
	}{
		{"hold inside reach", 99, true, false, true, 1},
		{"hold at reach", 100, true, false, true, 0},
		{"walking cast at reach", 100, true, true, true, 1},
		{"refused by the conditions", 50, true, true, false, 0},
		{"conditions skipped", 50, false, true, false, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newFolkCastRig(t, tc.dist)
			r.castAI.canDesire = tc.canDo
			r.f.AddCastDesire(r.target, modelskill.Ref{ID: 4126, Level: 1}, 1e6, tc.check, tc.move)
			r.in.Run()
			if got := r.f.cast.desires.Len(); got != tc.want {
				t.Fatalf("queued desires = %d, want %d", got, tc.want)
			}
			if d, ok := r.f.cast.desires.Peek(); ok && d.MoveToTarget != tc.move {
				t.Fatalf("desire MoveToTarget = %v, want %v", d.MoveToTarget, tc.move)
			}
		})
	}
}
