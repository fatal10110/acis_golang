package items

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestFlyingPlayerClickOnGroundItemAnswersActionFailed pins the flight gate
// that opens ItemInstance.onAction, ahead of the mercenary ticket gates and
// the pickup: a player riding a wyvern who clicks an ordinary ground item or
// a mercenary ticket is answered ActionFailed alone. Neither Gludio's lord,
// who may otherwise pick the ticket up, nor the game master, who would
// otherwise be told the ticket is not his castle's mercenary, picks anything
// up; both items stay on the ground and every inventory is unchanged.
func TestFlyingPlayerClickOnGroundItemAnswersActionFailed(t *testing.T) {
	t.Parallel()
	w := bootTicketWorld(t, ticketSetup{ownsCastle: 1})
	cleric := w.srv.GiveItem(t, w.leaderID, gludioClericTicket, 2)
	w.enter(t)

	x, y, z := w.srv.PlayerPosition(t, w.leaderID)
	w.srv.SeedGroundItem(t, 0, item.AdenaID, 5, x, y, z)
	drainUntilQuiet(t, w.leader)
	drainUntilQuiet(t, w.gm)
	adena := soleGroundObjectID(t, w.srv)
	ticket, _, ok := droppedGround(w.useTicket(t, cleric))
	if !ok {
		t.Fatal("no ticket placed")
	}
	drainUntilQuiet(t, w.gm)

	gm, ok := w.srv.State.PlayerByName("Admin")
	if !ok {
		t.Fatal("game master not online")
	}
	for _, rider := range []struct {
		name   string
		client *testsupport.ScriptedClient
		id     int32
	}{
		{"lord", w.leader, w.leaderID},
		{"game master", w.gm, gm.ObjectID()},
	} {
		adenaHeld := carriedCount(t, w.srv, rider.id, item.AdenaID)
		ticketsHeld := carriedCount(t, w.srv, rider.id, gludioClericTicket)
		mount := liveMount(t, w, rider.id)
		if !mount.Mount(wyvernMountNPCID, 1) {
			t.Fatalf("%s did not mount the wyvern", rider.name)
		}
		if flier, ok := mount.(interface{ Flying() bool }); !ok || !flier.Flying() {
			t.Fatalf("%s on a wyvern is not flying", rider.name)
		}
		drainUntilQuiet(t, rider.client)

		for _, target := range []struct {
			what string
			id   int32
		}{{"adena", adena}, {"mercenary ticket", ticket}} {
			rider.client.Send(encodeAction(target.id, 0, 0, 0, false))
			frames := collectFrames(rider.client)
			if len(frames) != 1 || frames[0][0] != serverpackets.OpcodeActionFailed {
				t.Fatalf("flying %s's click on the %s: opcodes %x, want ActionFailed alone", rider.name, target.what, opcodesOf(frames))
			}
			if _, ok := w.srv.State.Object(target.id); !ok {
				t.Fatalf("flying %s's click took the %s off the ground", rider.name, target.what)
			}
		}
		if got := carriedCount(t, w.srv, rider.id, item.AdenaID); got != adenaHeld {
			t.Fatalf("flying %s holds %d adena, want %d", rider.name, got, adenaHeld)
		}
		if got := carriedCount(t, w.srv, rider.id, gludioClericTicket); got != ticketsHeld {
			t.Fatalf("flying %s holds %d cleric tickets, want %d", rider.name, got, ticketsHeld)
		}
	}
	if got := gludioDropped(t, w.srv); got != 1 {
		t.Fatalf("Gludio dropped tickets = %d, want the one placed", got)
	}
}
