package summon

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable/attackabletest"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// duelOwner is a summon owner in duel duelID, 0 for none.
type duelOwner struct {
	fakeSummonOwner
	duelID int32
}

func (o *duelOwner) DuelID() int32 { return o.duelID }

// duelHitter is a player attacker whose duel a hit may interrupt.
type duelHitter struct {
	attackabletest.Combatant
	world.Presence
	id          int32
	duelID      int32
	interrupted bool
}

func (h *duelHitter) ObjectID() int32           { return h.id }
func (*duelHitter) Kind() actor.Kind            { return actor.KindPlayer }
func (h *duelHitter) Position() (int, int, int) { return 1000, 1000, 0 }
func (h *duelHitter) DuelID() int32             { return h.duelID }
func (h *duelHitter) InterruptDuel()            { h.interrupted = true }

// summonHitter is a summon whose owner acts through its hits.
type summonHitter struct {
	attackabletest.Combatant
	world.Presence
	owner *duelHitter
}

func (*summonHitter) Kind() actor.Kind { return actor.KindSummon }
func (s *summonHitter) Owner() (attackable.Combatant, bool) {
	return s.owner, true
}

// TestSummonHitInterruptsAttackerDuel pins SummonStatus.reduceHp's duel
// check (SummonStatus.java:32-38): any hit on a summon, damage or not,
// interrupts the duel of the player acting through the attacker unless the
// summon's owner is in that same duel. Ownerless summons interrupt every
// attacker, as the reference's null owner does.
func TestSummonHitInterruptsAttackerDuel(t *testing.T) {
	cases := []struct {
		name          string
		ownerDuel     int32
		ownerless     bool
		attackerDuel  int32
		throughSummon bool
		want          bool
	}{
		{name: "opponent's summon", ownerDuel: 1, attackerDuel: 1},
		{name: "outsider's summon", attackerDuel: 1, want: true},
		{name: "another duel's summon", ownerDuel: 2, attackerDuel: 1, want: true},
		{name: "outsiders", ownerDuel: 0, attackerDuel: 0},
		{name: "outsider's summon hit by a duellist's summon", attackerDuel: 1, throughSummon: true, want: true},
		{name: "opponent's summon hit by a duellist's summon", ownerDuel: 1, attackerDuel: 1, throughSummon: true},
		{name: "ownerless summon", ownerless: true, attackerDuel: 1, want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := ServitorConfig{ObjectID: 7, Stats: CombatStats{MaxHP: 1000}}
			if !tc.ownerless {
				cfg.Owner = &duelOwner{fakeSummonOwner: fakeSummonOwner{id: 42}, duelID: tc.ownerDuel}
			}
			s := mustServitor(t, cfg)
			hitter := &duelHitter{id: 5, duelID: tc.attackerDuel}
			var attacker attackable.Combatant = hitter
			if tc.throughSummon {
				attacker = &summonHitter{owner: hitter}
			}
			s.TakeDamage(0, attacker)
			if hitter.interrupted != tc.want {
				t.Fatalf("attacker's duel interrupted = %v, want %v", hitter.interrupted, tc.want)
			}
		})
	}
}
