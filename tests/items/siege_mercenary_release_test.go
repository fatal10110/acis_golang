package items

import (
	"testing"
)

// A ticket a siege start or the castle owner's removal passes over while a
// pickup holds it stays counted on the castle's grounds, so a pickup that
// then fails puts it back counted (#5249, follow-up of the owner-approved
// #5141 fix).

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

// TestOwnerRemovalKeepsCountingAHeldTicket holds the lord's ticket while
// the castle loses its owner, then lets go: the ticket is still on the
// ground and Gludio still counts it.
func TestOwnerRemovalKeepsCountingAHeldTicket(t *testing.T) {
	t.Parallel()
	w, ticket := bootGuardWorld(t)
	groundID, _, _ := w.placeTicket(t, ticket)
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
		t.Fatalf("Gludio dropped tickets after the pickup let go = %d, want 1", got)
	}
}
