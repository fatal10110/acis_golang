package npcs

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// Reference: ItemContainer.transferItem (ItemContainer.java:255-305) grows
// the target's stack with changeCount when the target already holds one, so
// no ItemInstance and no object id is created; only a partial move into a
// target holding no stack creates one through addItem(itemId, count).

// TestWarehouseMoveOntoHeldStackTakesNoObjectID deposits potions into the
// private warehouse in three moves and withdraws them in two. Only the
// split that starts the warehouse stack and the split that starts the
// inventory stack again take an object id; every move onto a held stack,
// split or whole, takes none, and the rows end where the moves left them.
func TestWarehouseMoveOntoHeldStackTakesNoObjectID(t *testing.T) {
	t.Parallel()
	w := bootKeeper(t, whAdena, [][2]int32{{potionID, 10}})
	potion := w.items[potionID]

	deposit := func(count int32, wantIDs int) {
		t.Helper()
		w.command(t, "DepositP")
		before := w.srv.IssuedObjectIDs()
		w.c.Send(encodeDeposit(whRow{potion, count}))
		drainFrames(t, w.c)
		if got := w.srv.IssuedObjectIDs() - before; got != wantIDs {
			t.Fatalf("depositing %d potions took %d object ids, want %d", count, got, wantIDs)
		}
	}
	deposit(4, 1) // splits into a warehouse holding none
	deposit(3, 0) // splits onto the warehouse stack
	deposit(3, 0) // moves the rest whole onto it
	if got := w.held(t, potion); got != 0 {
		t.Fatalf("potions still carried = %d, want 0", got)
	}
	db := saved(t, w.srv, w.player)
	if _, ok := db[potion]; ok {
		t.Fatalf("the emptied inventory potion row %d is still stored", potion)
	}
	var stored int32
	for objectID, row := range db {
		if row.templateID == potionID && row.loc == item.LocationWarehouse {
			if stored != 0 || row.count != 10 {
				t.Fatalf("saved warehouse potion rows = %+v, want one of 10", db)
			}
			stored = objectID
		}
	}
	if stored == 0 {
		t.Fatalf("saved rows = %+v, want 10 potions in the warehouse", db)
	}

	withdraw := func(count int32, wantIDs int) {
		t.Helper()
		w.command(t, "WithdrawP")
		before := w.srv.IssuedObjectIDs()
		w.c.Send(encodeWithdraw(whRow{stored, count}))
		drainFrames(t, w.c)
		if got := w.srv.IssuedObjectIDs() - before; got != wantIDs {
			t.Fatalf("withdrawing %d potions took %d object ids, want %d", count, got, wantIDs)
		}
	}
	withdraw(5, 1) // splits into an inventory holding none
	withdraw(5, 0) // moves the rest whole onto that stack
	db = saved(t, w.srv, w.player)
	if got := rowsAt(db, item.LocationWarehouse); len(got) != 0 {
		t.Fatalf("saved warehouse rows = %v, want none", got)
	}
	if got := rowsAt(db, item.LocationInventory); got[potionID] != 10 {
		t.Fatalf("saved inventory rows = %v, want 10 potions", got)
	}
	if got := w.srv.PlayerInventory(t, w.player).ItemsByTemplateID(potionID); len(got) != 1 || got[0].CountValue() != 10 {
		t.Fatalf("carried potion stacks = %d, want one of 10", len(got))
	}
}
