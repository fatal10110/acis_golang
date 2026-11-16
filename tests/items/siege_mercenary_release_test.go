package items

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// A ticket a siege start passes over while a pickup holds it stays counted
// on the castle's grounds, so a pickup that then fails puts it back
// counted; one the castle owner's removal passes over leaves the world when
// its pickup fails (#5249, follow-up of the owner-approved #5141 fix).

// TestSiegeStartKeepsCountingAHeldTicket holds the lord's ticket while the
// siege starts, then lets go as a failed pickup does: Gludio still counts
// it, and the owner's removal later decays it.
func TestSiegeStartKeepsCountingAHeldTicket(t *testing.T) {
	t.Parallel()
	w, ticket := bootGuardWorld(t)
	groundID, _, _ := w.placeTicket(t, ticket)
	ground := groundItem(t, w.srv, groundID)
	if !ground.Claim() {
		t.Fatal("placed ticket refused its first claim")
	}

	gludio := gludioSiegeOf(t, w)
	gludio.Start()
	drainUntilQuiet(t, w.leader)
	drainUntilQuiet(t, w.gm)
	if got := gludioDropped(t, w.srv); got != 1 {
		t.Fatalf("Gludio dropped tickets after the start = %d, want the held one", got)
	}
	ground.Release()
	if got := gludioDropped(t, w.srv); got != 1 {
		t.Fatalf("Gludio dropped tickets after the pickup let go = %d, want 1", got)
	}

	w.castleCommand(t, "remove gludio_castle")
	if _, ok := w.srv.State.Object(groundID); ok {
		t.Fatal("ticket still in the world after the castle's removal")
	}
	if got := gludioDropped(t, w.srv); got != 0 {
		t.Fatalf("Gludio dropped tickets after the castle's removal = %d, want 0", got)
	}
}

// TestSiegeStartHeldTicketPickedUp: the pickup that held the ticket through
// the siege start takes it, and Gludio no longer counts it.
func TestSiegeStartHeldTicketPickedUp(t *testing.T) {
	t.Parallel()
	w, ticket := bootGuardWorld(t)
	groundID, _, _ := w.placeTicket(t, ticket)
	ground := groundItem(t, w.srv, groundID)
	if !ground.Claim() {
		t.Fatal("placed ticket refused its first claim")
	}
	gludio := gludioSiegeOf(t, w)
	gludio.Start()
	drainUntilQuiet(t, w.leader)
	drainUntilQuiet(t, w.gm)
	ground.Release()

	w.leader.Send(encodeAction(groundID, 0, 0, 0, false))
	w.srv.SettlePickup(t, w.leaderID)
	drainUntilQuiet(t, w.leader)
	if n := carriedCount(t, w.srv, w.leaderID, gludioSwordTicket); n != 1 {
		t.Fatalf("tickets carried after the pickup = %d, want 1", n)
	}
	if got := gludioDropped(t, w.srv); got != 0 {
		t.Fatalf("Gludio dropped tickets after the pickup = %d, want 0", got)
	}
}

// TestOwnerRemovalDecaysAHeldTicketOnLetGo holds the lord's ticket while
// the castle loses its owner: the ticket stays on the ground, counted, until
// a pickup lets it go, and then leaves the world, so the castle's next owner
// neither counts it nor hires from it. The raw Claim and Release model the
// pickup in flight at the removal (its own let-go goes through the castle
// too); the let-go the castle sees is the next owner's refused pickup.
func TestOwnerRemovalDecaysAHeldTicketOnLetGo(t *testing.T) {
	t.Parallel()
	w, ticket := bootGuardWorld(t)
	groundID, x, y := w.placeTicket(t, ticket)
	ground := groundItem(t, w.srv, groundID)
	if !ground.Claim() {
		t.Fatal("placed ticket refused its first claim")
	}

	w.castleCommand(t, "remove gludio_castle")
	if _, ok := w.srv.State.Object(groundID); !ok {
		t.Fatal("a held ticket left the ground at the castle's removal")
	}
	ground.Release()
	if got := gludioDropped(t, w.srv); got != 1 {
		t.Fatalf("Gludio dropped tickets while the pickup held it = %d, want 1", got)
	}

	w.castleCommand(t, "set gludio_castle")
	drainUntilQuiet(t, w.leader)
	drainUntilQuiet(t, w.gm)
	w.srv.SetInventorySlotLimit(t, w.leaderID, 0)
	_, _, z := ground.Position()
	w.leader.Send(encodeAction(groundID, x, y, int32(z), false))
	assertFrameOpcode(t, w.leader.Read(), serverpackets.OpcodeActionFailed, "slots-full lead")
	assertStaticSystemMessage(t, w.leader.Read(), serverpackets.SystemMessageSlotsFull)
	drainUntilQuiet(t, w.leader)
	if _, ok := w.srv.State.Object(groundID); ok {
		t.Fatal("the ticket stayed on the ground after the pickup let it go")
	}
	if got := gludioDropped(t, w.srv); got != 0 {
		t.Fatalf("Gludio dropped tickets after the let-go = %d, want 0", got)
	}
	if n := carriedCount(t, w.srv, w.leaderID, gludioSwordTicket); n != 0 {
		t.Fatalf("tickets carried after the refused pickup = %d, want 0", n)
	}
}
