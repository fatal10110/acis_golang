package ai

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// TestAttackableCycleRunsWhileAttackingOrCasting pins the periodic step's
// stance switch: on every third step, after the weight decay, an actor
// whose current intention is ATTACK or CAST switches to run stance; one
// doing anything else does not.
func TestAttackableCycleRunsWhileAttackingOrCasting(t *testing.T) {
	for _, tc := range []struct {
		name    string
		desire  func(owner, target *fakeActor) *Desire
		current Intention
		want    int
	}{
		{"attack", func(_, target *fakeActor) *Desire {
			return &Desire{Kind: IntentionAttack, FinalTarget: target, Weight: 20}
		}, IntentionAttack, 1},
		{"cast", func(owner, _ *fakeActor) *Desire {
			return &Desire{Kind: IntentionCast, FinalTarget: owner, Skill: skill.Ref{ID: 4, Level: 1}, Weight: 10}
		}, IntentionCast, 1},
		{"idle", nil, IntentionIdle, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			owner := actor(1)
			target := actor(2)
			owner.known = map[int32]bool{owner.ObjectID(): true, target.ObjectID(): true}
			brain := NewAttackable(owner, &recordingMove{}, &recordingAttack{canAttack: true})
			pastFirstCycle(t, brain)
			brain.SetCastController(&recordingCast{canAttempt: true, canCast: true, stopsMove: true})
			if tc.desire != nil {
				brain.AddDamageHate(target, 0, 20)
				brain.Desires().AddOrUpdate(tc.desire(owner, target))
				if err := brain.RunAI(); err != nil {
					t.Fatalf("RunAI() error: %v", err)
				}
			}
			if got := brain.CurrentIntention(); got != tc.current {
				t.Fatalf("CurrentIntention() = %v, want %v", got, tc.current)
			}
			owner.runStanceCalls = 0
			for step := 1; step <= 3; step++ {
				brain.Tick()
				if err := brain.TickThink(); err != nil {
					t.Fatalf("TickThink() step %d error: %v", step, err)
				}
				want := 0
				if step == 3 {
					want = tc.want
				}
				if owner.runStanceCalls != want {
					t.Fatalf("run stance switches after step %d = %d, want %d", step, owner.runStanceCalls, want)
				}
			}
		})
	}
}
