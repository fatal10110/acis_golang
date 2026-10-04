package npcs

import (
	"bytes"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/derby"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestDerbyTicketRefusedWithoutTheAdena pins a ticket the better cannot pay
// for: only the not-enough-adena notice and the dispatcher's ActionFailed
// answer it, no ticket is handed over, nothing is staked in memory or in
// mdt_bets, and the lane and price picked are kept: the summary's Confirm
// buys that ticket once the adena is there.
func TestDerbyTicketRefusedWithoutTheAdena(t *testing.T) {
	w := bootDerby(t, 50)
	w.tick(1)
	w.manager = w.spawnFolk(t, folkTemplate("DerbyTrackManagerNpc", raceManagerID), 60)
	w.talkTo(t, w.manager)
	for _, step := range []string{"BuyTicket 0", "BuyTicket 5", "BuyTicket 10", "BuyTicket 13", "BuyTicket 20"} {
		html(t, w.dialog(t, step))
	}

	got := w.dialog(t, "BuyTicket 21")
	want := [][]byte{sysMsg(serverpackets.SystemMessageYouNotEnoughAdena), {serverpackets.OpcodeActionFailed}}
	if len(got) != len(want) {
		t.Fatalf("unpaid ticket answered %x, want not-enough-adena then ActionFailed", opcodes(got))
	}
	for i := range want {
		if !bytes.Equal(got[i], want[i]) {
			t.Fatalf("unpaid ticket frame %d = %x, want %x", i, got[i], want[i])
		}
	}
	if n := len(w.tickets(t)); n != 0 {
		t.Fatalf("held %d tickets after an unpaid purchase, want 0", n)
	}
	if got := w.held(t, item.AdenaID); got != 50 {
		t.Fatalf("adena after an unpaid purchase = %d, want 50", got)
	}
	if got := w.track.Stakes(); got != ([derby.Lanes]int64{}) {
		t.Fatalf("stakes after an unpaid purchase = %v, want none", got)
	}
	stored := storedStakes(t, w.srv)
	if len(stored) != derby.Lanes {
		t.Fatalf("stored %d lanes, want the %d seeded", len(stored), derby.Lanes)
	}
	for lane, bet := range stored {
		if bet != 0 {
			t.Fatalf("stored stake on lane %d = %d after an unpaid purchase, want 0", lane, bet)
		}
	}

	// The summary stays open with the lane and price picked: once paid
	// for, Confirm buys that very ticket.
	w.onPlayer(t, func(pc *player.Character) {
		if pc.Inventory().AddNew(item.AdenaID, 1000, 0x7f000001) == nil {
			t.Error("adena not added")
		}
	})
	drainFrames(t, w.c)
	w.assertChat(t, "confirm once paid", w.dialog(t, "BuyTicket 21"),
		sysMsg(serverpackets.SystemMessageS1DisappearedAdena, numberParam(1000)),
		sysMsg(serverpackets.SystemMessageAcquiredS1S2, numberParam(1), itemNameParam(derby.TicketItemID)))
	tickets := w.tickets(t)
	if len(tickets) != 1 {
		t.Fatalf("held %d tickets after paying, want 1", len(tickets))
	}
	if st := tickets[0].Snapshot(); st.EnchantLevel != 1 || st.CustomType1 != 5 || st.CustomType2 != 10 {
		t.Fatalf("ticket = race %d lane %d price %d00, want race 1 lane 5 price 1000", st.EnchantLevel, st.CustomType1, st.CustomType2)
	}
	if got := w.track.Stakes(); got != ([derby.Lanes]int64{4: 1000}) {
		t.Fatalf("stakes after paying = %v, want 1000 on lane 5", got)
	}
}
