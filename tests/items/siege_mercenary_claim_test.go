package items

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The siege start claims each dropped ticket on the ground before it hires
// the ticket's mercenary, so a pickup that holds the ticket keeps it and no
// mercenary is hired for it (#5141, an owner-approved divergence: the
// reference hires first and decays the ticket after, without a claim).

// gludioMercenaries takes the mercenaries Gludio Castle recorded hiring.
func gludioMercenaries(t *testing.T, srv *gameservertest.Server) []int32 {
	t.Helper()
	c, ok := srv.Castles.Get(1)
	if !ok {
		t.Fatal("no Gludio Castle")
	}
	return c.TakeMercenaries()
}

// TestSiegeStartSkipsClaimedTicket holds the lord's ticket with a pickup's
// claim while the siege starts: no mercenary spawns or is recorded and the
// ticket stays on the ground. The pickup then completes and the lord holds
// the ticket, with still no mercenary.
func TestSiegeStartSkipsClaimedTicket(t *testing.T) {
	t.Parallel()
	w, ticket := bootGuardWorld(t)
	groundID, _, _ := w.placeTicket(t, ticket)
	ground := groundItem(t, w.srv, groundID)
	if !ground.Claim() {
		t.Fatal("placed ticket refused its first claim")
	}

	gludio := gludioSiegeOf(t, w)
	gludio.Start()
	if !gludio.InProgress() {
		t.Fatal("siege not under way")
	}
	drainUntilQuiet(t, w.leader)
	drainUntilQuiet(t, w.gm)
	if got := w.guardCount(); got != [3]int{0, 0, 1} {
		t.Fatalf("guards at the start [mercenary, ownerless, owner's] = %v, want [0 0 1]", got)
	}
	if _, ok := w.srv.State.Object(groundID); !ok {
		t.Fatal("a claimed ticket left the ground at the siege start")
	}

	// The pickup holding the ticket lets go and the lord takes it.
	ground.Release()
	w.leader.Send(encodeAction(groundID, 0, 0, 0, false))
	w.srv.SettlePickup(t, w.leaderID)
	drainUntilQuiet(t, w.leader)
	if _, ok := w.srv.State.Object(groundID); ok {
		t.Fatal("picked-up ticket is still on the ground")
	}
	if n := carriedCount(t, w.srv, w.leaderID, gludioSwordTicket); n != 1 {
		t.Fatalf("tickets carried after the pickup = %d, want 1", n)
	}
	if got := w.guardCount(); got[0] != 0 {
		t.Fatalf("mercenaries after the pickup = %d, want 0", got[0])
	}
	if got := gludioMercenaries(t, w.srv); len(got) != 0 {
		t.Fatalf("Gludio recorded mercenaries %v, want none", got)
	}
}

// TestSiegeStartRacesTicketPickup sends the lord's pickup of its ticket and
// starts the siege at once: whichever claims the ticket first wins, so the
// lord ends with the ticket or a mercenary stands for it, never both, and
// the castle records exactly the mercenaries in the world.
func TestSiegeStartRacesTicketPickup(t *testing.T) {
	t.Parallel()
	w, ticket := bootGuardWorld(t)
	groundID, _, _ := w.placeTicket(t, ticket)

	gludio := gludioSiegeOf(t, w)
	w.leader.Send(encodeAction(groundID, 0, 0, 0, false))
	gludio.Start()
	drainUntilQuiet(t, w.leader)
	w.srv.SettlePickup(t, w.leaderID)
	drainUntilQuiet(t, w.leader)
	drainUntilQuiet(t, w.gm)

	if _, ok := w.srv.State.Object(groundID); ok {
		t.Fatal("ticket still on the ground after the pickup and the siege start")
	}
	carried := carriedCount(t, w.srv, w.leaderID, gludioSwordTicket)
	mercs := w.guardCount()[0]
	if carried+mercs != 1 {
		t.Fatalf("tickets carried %d, mercenaries %d: want exactly one of them", carried, mercs)
	}
	if got := gludioMercenaries(t, w.srv); len(got) != mercs {
		t.Fatalf("Gludio recorded mercenaries %v, want %d", got, mercs)
	}
}
