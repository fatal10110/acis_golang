package ai

import (
	"testing"

	modelactor "github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
)

// competitorFake is a gate fake with an Olympiad standing; a summon's
// owner may be one too.
type competitorFake struct {
	*gateFake
	game    int
	started bool
	owner   attackable.Combatant
}

func (c *competitorFake) OlympiadStarted() bool { return c.started }
func (c *competitorFake) OlympiadGameID() int   { return c.game }

func (c *competitorFake) Owner() (attackable.Combatant, bool) {
	if c.owner != nil {
		return c.owner, true
	}
	return c.gateFake.Owner()
}

func competitor(id int32, game int, started bool) *competitorFake {
	return &competitorFake{gateFake: gatePlayerFake(id, 40, 0), game: game, started: started}
}

// TestCanKeepAttackingOlympiadOpponent pins Playable.canKeepAttacking's
// Olympiad branch: an unflagged target is kept while its acting player
// fights the attacker's in the same started match, a summon acting for its
// owner, and dropped before the fight starts or across stadiums.
func TestCanKeepAttackingOlympiadOpponent(t *testing.T) {
	tests := []struct {
		name     string
		attacker attackable.Combatant
		target   attackable.Combatant
		want     bool
	}{
		{"same match", competitor(1, 2, true), competitor(2, 2, true), true},
		{"not started", competitor(1, 2, false), competitor(2, 2, false), false},
		{"another stadium", competitor(1, 2, true), competitor(2, 3, true), false},
		{"own summon at the opponent", &competitorFake{gateFake: &gateFake{id: 3, kind: modelactor.KindSummon, level: 1}, owner: competitor(1, 2, true)}, competitor(2, 2, true), true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := canKeepAttacking(tc.attacker, tc.target); got != tc.want {
				t.Fatalf("canKeepAttacking() = %v, want %v", got, tc.want)
			}
		})
	}
}
