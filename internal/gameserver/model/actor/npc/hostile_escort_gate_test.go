package npc

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// deathTarget is a follow target whose death state the test sets.
type deathTarget struct {
	*hostileTarget
	dead, alikeDead bool
}

func (d deathTarget) Dead() bool      { return d.dead }
func (d deathTarget) AlikeDead() bool { return d.alikeDead }

// TestThinkFollowDropsOnlyADeadTarget pins the follow step's target gate: a
// target faking death keeps the follow desire, a dead one drops it.
func TestThinkFollowDropsOnlyADeadTarget(t *testing.T) {
	for _, tc := range []struct {
		name            string
		dead, alikeDead bool
		wantClear       bool
	}{
		{"fake death", false, true, false},
		{"dead", true, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			follower := partyHostile(t, 1, 0, &hostileMove{})
			state := world.New()
			follower.Attach(Runtime{World: state})
			state.Spawn(follower, 0, 0, 0, 0)
			follower.roll = func(int) int { return 0 }
			target := deathTarget{hostileTarget: &hostileTarget{id: 2}, dead: tc.dead, alikeDead: tc.alikeDead}
			if got := follower.ThinkFollow(target, false); got != tc.wantClear {
				t.Fatalf("ThinkFollow() clearDesire = %v, want %v", got, tc.wantClear)
			}
		})
	}
}

// TestLooseFollowMovesFromRadius pins the loose follow's radius: a follower
// 150 or more from its target walks toward it, one at 149 stays.
func TestLooseFollowMovesFromRadius(t *testing.T) {
	for _, tc := range []struct {
		distance  int
		wantMoves int
	}{
		{149, 0},
		{150, 1},
		{151, 1},
	} {
		move := &hostileMove{}
		follower := partyHostile(t, 1, 0, move)
		target := partyHostile(t, 2, 0, &hostileMove{})
		state := world.New()
		follower.Attach(Runtime{World: state})
		target.Attach(Runtime{World: state})
		state.Spawn(follower, 0, 0, 0, 0)
		state.Spawn(target, tc.distance, 0, 0, 0)
		rolls := []int{51, 250000, 0}
		follower.roll = func(int) int {
			if len(rolls) == 0 {
				t.Fatalf("distance %d: unexpected extra roll", tc.distance)
			}
			r := rolls[0]
			rolls = rolls[1:]
			return r
		}

		if follower.ThinkFollow(target, false) {
			t.Fatalf("distance %d: ThinkFollow() clearDesire = true, want follow kept", tc.distance)
		}
		if got := len(move.locations); got != tc.wantMoves {
			t.Fatalf("distance %d: loose-follow moves = %d, want %d", tc.distance, got, tc.wantMoves)
		}
	}
}
