package ai

import (
	"fmt"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
)

// wantFreedDeacon fails unless the deacon keeps no party, both barriers
// stand open and no garden zone admits anyone.
func (w *sirraWorld) wantFreedDeacon(t *testing.T, step string) {
	t.Helper()
	w.wantBarriers(t, step, true)
	if got, want := w.deaconMemory(t), "leader=0 busy=0 third=0 count=2 entrant=false"; got != want {
		t.Fatalf("%s: the deacon's memory = %s, want %s", step, got, want)
	}
	for _, z := range w.zones {
		if allowed := z.AllowedPlayers(); len(allowed) != 0 {
			t.Fatalf("%s: the zones admit %v, want none", step, allowed)
		}
	}
}

// wantPaid fails unless the leader leaderID holds no Silver Hemocyte and
// its three entry rewards: the payment stays taken whatever the visit
// becomes.
func (w *sirraWorld) wantPaid(t *testing.T, step string, leaderID int32) {
	t.Helper()
	if got := w.srv.PlayerItemCount(t, leaderID, sirraHemocyte); got != 0 {
		t.Fatalf("%s: the leader holds %d Silver Hemocytes, want 0", step, got)
	}
	if got := w.srv.PlayerItemCount(t, leaderID, sirraReward); got != 3 {
		t.Fatalf("%s: the leader holds %d of item 8379, want its 3", step, got)
	}
}

// TestFreyaDeaconGateOutlivesTheLeadersLogout: the leader pays for a party
// of two and logs out inside the two-minute wait. The gate timer belongs to
// the visit, so it still runs: two minutes after the entry the memo's
// creature hears 10027 and both barriers close; 5 s later the gate finds
// the leader gone from any party, lets no one in, frees the deacon and
// opens both barriers. The payment, taken at the entry, is not given back.
// Mate, left alone, finds
// the deacon free: its talk answers 001 and its entry asks for a party
// (06), not the busy page (01). An owner-approved divergence (#5130).
func TestFreyaDeaconGateOutlivesTheLeadersLogout(t *testing.T) {
	w := bootSirra(t, 10)
	c := w.srv.Client
	leaderID := w.player.ObjectID()
	mate := w.join(t, "mate", "Mate", 10)
	w.wantFirstPage(t, c, "freya_deacon001.htm")
	w.bypass(t, c, "enter")
	w.srv.ReadQueued(t, mate.c)
	w.wantPaid(t, "the entry", leaderID)

	c.Send(wire.NewPacketWriter(clientpackets.OpcodeLogout).Bytes())
	w.srv.AwaitConnectionEnded(t, c)
	w.srv.ReadQueued(t, mate.c)
	if got, want := w.deaconMemory(t), fmt.Sprintf("leader=%d busy=1 third=0 count=2 entrant=true", leaderID); got != want {
		t.Fatalf("the deacon's memory after the logout = %s, want %s", got, want)
	}

	w.srv.Advance(t, 120*time.Second)
	w.log.expect(t, "two minutes after the entry", "memo heard 10027 0 0")
	w.wantBarriers(t, "two minutes after the entry", false)
	w.srv.Advance(t, 5*time.Second)
	w.log.expect(t, "the gate")
	w.wantFreedDeacon(t, "the gate")

	w.srv.ReadQueued(t, mate.c)
	w.wantFirstPage(t, mate.c, "freya_deacon001.htm")
	if got := baiumPageNames(w.bypass(t, mate.c, "enter")); len(got) != 1 || got[0] != "freya_deacon_q0656_06.htm" {
		t.Fatalf("Mate's entry after the gate: pages = %q, want freya_deacon_q0656_06.htm", got)
	}
}

// TestFreyaDeaconGateFreesABrokenParty: Mate leaves the party after its
// leader paid. Two minutes after the entry both barriers close; 5 s later
// the gate finds the leader in no party, lets no one in, frees the deacon
// and opens both barriers. The payment stays taken and the deacon greets
// the leader as free (001). An owner-approved divergence (#5130).
func TestFreyaDeaconGateFreesABrokenParty(t *testing.T) {
	w := bootSirra(t, 10)
	c := w.srv.Client
	leaderID := w.player.ObjectID()
	mate := w.join(t, "mate", "Mate", 10)
	w.wantFirstPage(t, c, "freya_deacon001.htm")
	w.bypass(t, c, "enter")
	w.srv.ReadQueued(t, mate.c)

	mate.c.Send(wire.NewPacketWriter(clientpackets.OpcodeRequestWithdrawParty).Bytes())
	w.srv.Settle(t)
	w.srv.ReadQueued(t, mate.c)
	w.srv.ReadQueued(t, c)

	w.advance(t, 120*time.Second)
	w.log.expect(t, "two minutes after the entry", "memo heard 10027 0 0")
	w.wantBarriers(t, "two minutes after the entry", false)
	if frames := w.advance(t, 5*time.Second); len(baiumPageNames(frames)) != 0 {
		t.Fatalf("the gate sent the leader pages %q", baiumPageNames(frames))
	}
	w.log.expect(t, "the gate")
	w.wantFreedDeacon(t, "the gate")
	w.wantPaid(t, "the gate", leaderID)
	w.wantFirstPage(t, c, "freya_deacon001.htm")
}
