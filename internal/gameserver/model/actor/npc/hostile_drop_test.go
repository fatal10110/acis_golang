package npc

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
)

// dropRecorder is death rewards that record the drops asked of them.
type dropRecorder struct{ drops [][2]int32 }

func (*dropRecorder) CalculateRewards(attackable.Combatant) {}
func (d *dropRecorder) DropItem(_ attackable.Combatant, itemID int32, count int) {
	d.drops = append(d.drops, [2]int32{itemID, int32(count)})
}

// TestHostileDropItemGoesToItsRewards: DropItem hands the drop to the
// rewards the NPC was attached with, and panics on an NPC attached with
// none rather than dropping nothing.
func TestHostileDropItemGoesToItsRewards(t *testing.T) {
	rec := &dropRecorder{}
	h := &Hostile{rewards: rec}
	h.DropItem(nil, 6353, 1)
	if len(rec.drops) != 1 || rec.drops[0] != [2]int32{6353, 1} {
		t.Fatalf("drops = %v, want [[6353 1]]", rec.drops)
	}

	defer func() {
		if recover() == nil {
			t.Fatal("DropItem on a hostile with no rewards did not panic")
		}
	}()
	(&Hostile{}).DropItem(nil, 6353, 1)
}
