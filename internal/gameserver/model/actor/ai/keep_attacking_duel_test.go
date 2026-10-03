package ai

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/duel"
	modelactor "github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
)

// duelGateFake is a gate fake with a duel standing; a summon's owner may
// be one too.
type duelGateFake struct {
	*gateFake
	duelID int32
	state  duel.State
	owner  attackable.Combatant
}

func (d *duelGateFake) DuelID() int32         { return d.duelID }
func (d *duelGateFake) DuelState() duel.State { return d.state }

func (d *duelGateFake) Owner() (attackable.Combatant, bool) {
	if d.owner != nil {
		return d.owner, true
	}
	return d.gateFake.Owner()
}

func duellistFake(id, duelID int32, state duel.State) *duelGateFake {
	return &duelGateFake{gateFake: gatePlayerFake(id, 40, 0), duelID: duelID, state: state}
}

func duelSummonFake(id int32, owner *duelGateFake) *duelGateFake {
	return &duelGateFake{gateFake: &gateFake{id: id, kind: modelactor.KindSummon, level: 1}, owner: owner}
}

// TestCanKeepAttackingDuelOpponent pins Playable.canKeepAttacking's duel
// branch: an unflagged target is kept while its acting player fights the
// attacker's in the same duel, a summon on either side acting for its
// owner, and is dropped once either is out of the fight or in another duel.
func TestCanKeepAttackingDuelOpponent(t *testing.T) {
	tests := []struct {
		name     string
		attacker attackable.Combatant
		target   attackable.Combatant
		want     bool
	}{
		{"same duel", duellistFake(1, 7, duel.Duelling), duellistFake(2, 7, duel.Duelling), true},
		{"another duel", duellistFake(1, 7, duel.Duelling), duellistFake(2, 8, duel.Duelling), false},
		{"defeated opponent", duellistFake(1, 7, duel.Duelling), duellistFake(2, 7, duel.Dead), false},
		{"counting down", duellistFake(1, 7, duel.Countdown), duellistFake(2, 7, duel.Countdown), false},
		{"opponent's summon", duellistFake(1, 7, duel.Duelling), duelSummonFake(3, duellistFake(2, 7, duel.Duelling)), true},
		{"own summon at the opponent", duelSummonFake(3, duellistFake(1, 7, duel.Duelling)), duellistFake(2, 7, duel.Duelling), true},
		{"outsider's summon", duellistFake(1, 7, duel.Duelling), duelSummonFake(3, duellistFake(2, 0, duel.NoDuel)), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := canKeepAttacking(tc.attacker, tc.target); got != tc.want {
				t.Fatalf("canKeepAttacking() = %v, want %v", got, tc.want)
			}
		})
	}
}
