package boat

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestPassengerTeleportedAwayKeepsFareForOthers pins the ticket collection
// surviving one passenger teleported off the boat before it runs (owner
// decision, #5154): the passenger teleported away keeps its ticket, and the
// one still aboard is charged its ticket when the collection runs.
func TestPassengerTeleportedAwayKeepsFareForOthers(t *testing.T) {
	t.Parallel()
	srv, gm, me := bootReload(t, nil, &atomic.Bool{}, passengerItinerary())
	b := srv.Boats.Boats()[0]
	rider, riderID := addRider(t, srv, "rider", "Rider", runeBoarding)
	board(t, srv, gm, b.ObjectID())
	board(t, srv, rider, b.ObjectID())

	sail(srv, 305)
	srv.ReadQueued(t, gm)
	srv.ReadQueued(t, rider)
	gm.Send(encodeBuildCmd(fmt.Sprintf("teleport %d %d %d", reloadTown.X, reloadTown.Y, reloadTown.Z)))
	srv.ReadQueued(t, gm)
	if got := b.Passengers(); !slices.Equal(got, []int32{riderID}) {
		t.Fatalf("passengers after the teleport %v, want only %d", got, riderID)
	}

	// The fare falls due at 308.0, after that second's last update.
	sail(srv, 3)
	rest, _ := withoutChecks(passengerLog(t, srv, rider))
	var own []string
	for _, s := range rest {
		if !strings.Contains(s, fmt.Sprint(" ", me, " ")) {
			own = append(own, s)
		}
	}
	assertLog(t, "the rider, second 308", own, sm(serverpackets.SystemMessageS1Disappeared))
	if log := passengerLog(t, srv, gm); slices.Contains(log, sm(serverpackets.SystemMessageS1Disappeared)) {
		t.Fatalf("the passenger teleported away was charged: %q", log)
	}
	if got := b.Passengers(); !slices.Equal(got, []int32{riderID}) {
		t.Fatalf("passengers after the fare %v, want the rider still aboard", got)
	}

	if n := srv.PlayerItemCount(t, riderID, ticketID); n != 0 {
		t.Fatalf("rider holds %d tickets, want the fare taken", n)
	}
	if n := srv.PlayerItemCount(t, me, ticketID); n != 1 {
		t.Fatalf("teleported passenger holds %d tickets, want 1", n)
	}
	srv.FlushItems(t)
	for id, want := range map[int32]int{riderID: 0, me: 1} {
		var stored int
		if err := srv.DB.QueryRowContext(context.Background(),
			"SELECT COALESCE(SUM(count), 0) FROM items WHERE owner_id = ? AND item_id = ?", id, ticketID).Scan(&stored); err != nil {
			t.Fatal(err)
		}
		if stored != want {
			t.Fatalf("stored tickets of %d: %d, want %d", id, stored, want)
		}
	}
}
