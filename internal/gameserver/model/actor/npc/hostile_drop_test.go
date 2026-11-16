package npc

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
)

// dropRecorder is an item dropper that records the drops asked of it.
type dropRecorder struct{ drops [][2]int32 }

func (d *dropRecorder) DropItem(_ *Hostile, _ attackable.Combatant, itemID int32, count int) {
	d.drops = append(d.drops, [2]int32{itemID, int32(count)})
}

// TestHostileDropItemGoesToItsDrops: DropItem hands the drop to the item
// dropper the NPC was attached with, whatever its death rewards, and drops
// nothing on an NPC attached with none rather than panicking.
func TestHostileDropItemGoesToItsDrops(t *testing.T) {
	h := newTestHostile(t, &hostileMove{}, &hostileAttack{})
	h.DropItem(nil, 6353, 1)

	rec := &dropRecorder{}
	h.Attach(Runtime{Drops: rec})
	h.DropItem(nil, 6353, 1)
	if len(rec.drops) != 1 || rec.drops[0] != [2]int32{6353, 1} {
		t.Fatalf("drops = %v, want [[6353 1]]", rec.drops)
	}
}
