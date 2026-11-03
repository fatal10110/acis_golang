package ai

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// The Partisan Hideaway's Agit01Move1 soldier and its sorcerer child.
const (
	partisanSoldierID   = int32(35372) // Agit01Move1
	partisanSorcerer2ID = int32(35380) // Agit01PartisanSorcerer2
)

// TestPartisanSoldiersAttackWhoSpells ports Agit01Move1's see-spell hook,
// which Agit01PartisanSorcerer2 inherits: the character's spell with 22
// aggro points on a monster beside them makes the soldier and the sorcerer
// each want to attack the character further by 22 / 440 / 0.05 * 150 =
// 150. They have seen the character first, so the spell is all that moves.
func TestPartisanSoldiersAttackWhoSpells(t *testing.T) {
	t.Parallel()
	w := bootHalls(t, 0, 0, location.Location{}, siegeSkillBoot(t),
		agitTemplate(partisanSoldierID), agitTemplate(partisanSorcerer2ID), warriorTemplate(unboundID, nil))
	dummy := w.spawn(t, unboundID, 150, 0)
	soldier := w.spawn(t, partisanSoldierID, 0, 150)
	sorcerer := w.spawn(t, partisanSorcerer2ID, 0, -150)
	w.ticks(t, 1)
	weight := func(h *npc.Hostile) float64 {
		d, _ := attackDesireOn(h, w.player.ObjectID())
		return d.Weight
	}
	before := map[*npc.Hostile]float64{soldier: weight(soldier), sorcerer: weight(sorcerer)}

	w.selectTarget(t, dummy)
	w.cast(t, siegeBoltID)
	for name, h := range map[string]*npc.Hostile{"soldier": soldier, "sorcerer": sorcerer} {
		if got := weight(h) - before[h]; got != 150 {
			t.Errorf("%s attack weight gained from the spell = %v, want 150", name, got)
		}
	}
}
