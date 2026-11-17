package npcs

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// TestWarehouseWithdrawZeroCountRowMovesNothing pins a withdrawal row naming
// 0 units: the decoder accepts it, and the transfer moves nothing and creates
// nothing, for a whole non-stackable item and a stack alike. The warehouse
// keeps its rows, the inventory gains none, and the client gets no answer.
func TestWarehouseWithdrawZeroCountRowMovesNothing(t *testing.T) {
	t.Parallel()
	w := bootKeeper(t, whAdena, [][2]int32{{swordID, 1}, {potionID, 10}})
	w.command(t, "DepositP")
	w.c.Send(encodeDeposit(whRow{w.items[swordID], 1}, whRow{w.items[potionID], 10}))
	drainUntilQuiet(t, w.c)
	w.command(t, "WithdrawP")

	db := saved(t, w.srv, w.player)
	var sword, potions int32
	for objectID, row := range db {
		if row.loc != item.LocationWarehouse {
			continue
		}
		switch row.templateID {
		case swordID:
			sword = objectID
		case potionID:
			potions = objectID
		}
	}
	if sword == 0 || potions == 0 {
		t.Fatalf("warehouse rows = %v, want the sword and the potions", db)
	}

	w.c.Send(encodeWithdraw(whRow{sword, 0}, whRow{potions, 0}))
	if frames := drainFrames(t, w.c); len(frames) != 0 {
		t.Fatalf("zero-count withdrawal answered %x, want nothing", opcodes(frames))
	}
	db = saved(t, w.srv, w.player)
	if got := rowsAt(db, item.LocationWarehouse); got[swordID] != 1 || got[potionID] != 10 || len(got) != 2 {
		t.Fatalf("saved warehouse rows = %v, want the sword and 10 potions kept", got)
	}
	if got := rowsAt(db, item.LocationInventory); got[swordID] != 0 || got[potionID] != 0 {
		t.Fatalf("saved inventory rows = %v, want no sword or potions", got)
	}
}
