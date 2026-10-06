package boat

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestFareShowsAndPersistsTicketTaken pins what the fare leaves behind
// beyond its system message: the passenger's client is sent an
// InventoryUpdate for the ticket taken, and the items row holds one ticket
// less.
func TestFareShowsAndPersistsTicketTaken(t *testing.T) {
	t.Parallel()
	srv, c, me, b := bootPassenger(t, runeBoarding, 2)
	c.Send(encodeMoveInVehicle(b, deckSpot, deckCenter))
	passengerLog(t, srv, c)
	c.Send(encodeGetOnVehicle(b, deckSpot))
	passengerLog(t, srv, c)

	// The boat sails at 303 and the fare falls due at 308.0.
	sail(srv, 308)
	srv.Settle(t)
	srv.InventoryUpdates.Tick()

	var updates int
	for _, f := range srv.ReadQueued(t, c) {
		if f[0] == serverpackets.OpcodeInventoryUpdate {
			updates++
		}
	}
	if updates != 1 {
		t.Fatalf("InventoryUpdate frames after the fare %d, want 1", updates)
	}
	if got := srv.PlayerInventory(t, me).ItemCount(ticketID, -1, false); got != 1 {
		t.Fatalf("tickets left %d, want 1", got)
	}

	srv.FlushItems(t)
	var stored int
	if err := srv.DB.QueryRowContext(context.Background(),
		"SELECT COALESCE(SUM(count), 0) FROM items WHERE owner_id = ? AND item_id = ?", me, ticketID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != 1 {
		t.Fatalf("stored tickets %d, want 1", stored)
	}
}
