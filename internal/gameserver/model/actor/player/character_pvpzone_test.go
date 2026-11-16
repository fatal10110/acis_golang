package player

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
)

// zonedSummon is a summon target that knows whether it stands in a PvP zone.
type zonedSummon struct {
	summonKiller
	inPvP bool
}

func (s zonedSummon) InPvPZone() bool { return s.inPvP }

// TestNotePvPAttackZoneExemptionFollowsTheHitCreature pins the PvP-zone
// exemption to the creature hit: an attacker in a PvP zone flags nothing
// only when that creature stands in one too, so a summon's zone decides for
// a hit on the summon whatever its owner's, and the owner's for a hit on the
// owner.
func TestNotePvPAttackZoneExemptionFollowsTheHitCreature(t *testing.T) {
	tests := []struct {
		name                 string
		ownerInPvP, hitInPvP bool
		hitSummon            bool
		wantFlag             bool
	}{
		{"summon inside, owner outside", false, true, true, false},
		{"summon outside, owner inside", true, false, true, true},
		{"owner inside", true, true, false, false},
		{"owner outside", false, false, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attacker := &Character{ID: 1}
			attacker.SetInPvPZone(true)
			owner := &Character{ID: 2}
			owner.SetInPvPZone(tt.ownerInPvP)
			var target attackable.Combatant = owner
			if tt.hitSummon {
				target = zonedSummon{summonKiller: summonKiller{owner: owner}, inPvP: tt.hitInPvP}
			}
			rec := recordEvents(attacker)

			attacker.NotePvPAttack(target)
			attacker.NotePvPSkillTargets([]attackable.Combatant{target}, true, "PDAM")

			want := 0
			if tt.wantFlag {
				want = 2
			}
			if got := len(pvpFlagCalls(rec)); got != want {
				t.Fatalf("PvP flags after a hit and an offensive skill = %d, want %d", got, want)
			}
		})
	}
}
