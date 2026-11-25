package npcs

import (
	"bytes"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/derby"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// pickTicket walks the purchase dialog to the summary of a ticket on lane
// at the price of price step (11 to 18).
func (w *derbyWorld) pickTicket(t *testing.T, lane, price string) {
	t.Helper()
	for _, step := range []string{"BuyTicket 0", "BuyTicket " + lane, "BuyTicket 10", "BuyTicket " + price, "BuyTicket 20"} {
		html(t, w.dialog(t, step))
	}
}

// TestDerbyTicketSoldAtTheCloseMakesTheOdds pins #5119 on both sides of the
// closing step: a ticket bought the second before sales close is paid,
// staked and counted in the odds, while a confirm sent after the close is
// refused with no adena taken, no ticket and no stake.
func TestDerbyTicketSoldAtTheCloseMakesTheOdds(t *testing.T) {
	w := bootDerby(t, 5000)
	w.tick(1)
	w.manager = w.spawnFolk(t, folkTemplate("DerbyTrackManagerNpc", raceManagerID), 60)
	w.talkTo(t, w.manager)

	w.pickTicket(t, "2", "11")
	w.assertChat(t, "early ticket", w.dialog(t, "BuyTicket 21"),
		sysMsg(serverpackets.SystemMessageS1DisappearedAdena, numberParam(100)),
		sysMsg(serverpackets.SystemMessageAcquiredS1S2, numberParam(1), itemNameParam(derby.TicketItemID)))

	// The closing step is next.
	w.tick(899)
	drainFrames(t, w.c)
	w.pickTicket(t, "5", "13")
	w.assertChat(t, "last-second ticket", w.dialog(t, "BuyTicket 21"),
		sysMsg(serverpackets.SystemMessageS1DisappearedAdena, numberParam(1000)),
		sysMsg(serverpackets.SystemMessageAcquiredS1S2, numberParam(1), itemNameParam(derby.TicketItemID)))
	w.pickTicket(t, "3", "11")

	w.tick(1)
	drainFrames(t, w.c)
	w.assertChat(t, "confirm once closed", w.dialog(t, "BuyTicket 21"), sysMsg(derby.MsgTicketsNotAvailable))

	if got := w.held(t, item.AdenaID); got != 3900 {
		t.Fatalf("adena = %d, want 3900: two tickets paid, the late one not", got)
	}
	if n := len(w.tickets(t)); n != 2 {
		t.Fatalf("held %d tickets, want 2", n)
	}
	if got := w.track.Stakes(); got != ([derby.Lanes]int64{1: 100, 4: 1000}) {
		t.Fatalf("stakes = %v, want 100 on lane 2 and 1000 on lane 5", got)
	}
	if stored := storedStakes(t, w.srv); stored[2] != 100 || stored[5] != 1000 || stored[3] != 0 {
		t.Fatalf("stored stakes = %v, want lane 2 100, lane 5 1000, lane 3 none", stored)
	}
	// 70% of the 1100 staked over each lane's stake, at least 1.25: the
	// last-second stake is in the pool.
	oddsPage := append(runnerNames(), "Odd1", "&$804;", "Odd2", "7.7", "Odd3", "&$804;", "Odd4", "&$804;", "Odd5", "1.3", "Odd6", "&$804;", "Odd7", "&$804;", "Odd8", "&$804;", "1race", "1")
	w.assertPage(t, "odds", w.dialog(t, "ShowOdds"), derbyPage(t, w.manager, "30995-5.htm", oddsPage...))
}

// TestDerbyTicketBoughtDuringTheClosingTick races a confirm against the
// closing step (#5119): whichever wins, the ticket is either sold, paid and
// counted in the odds, or refused with nothing taken.
func TestDerbyTicketBoughtDuringTheClosingTick(t *testing.T) {
	w := bootDerby(t, 5000)
	w.tick(1)
	w.manager = w.spawnFolk(t, folkTemplate("DerbyTrackManagerNpc", raceManagerID), 60)
	w.talkTo(t, w.manager)
	w.tick(899)
	drainFrames(t, w.c)
	w.pickTicket(t, "5", "13")

	ticked := make(chan struct{})
	go func() {
		w.track.Tick()
		close(ticked)
	}()
	got := w.dialog(t, "BuyTicket 21")
	<-ticked

	acquired := sysMsg(serverpackets.SystemMessageAcquiredS1S2, numberParam(1), itemNameParam(derby.TicketItemID))
	sold := false
	for _, f := range got {
		sold = sold || bytes.Equal(f, acquired)
	}
	wantAdena, wantStakes, wantOdd5 := 5000, [derby.Lanes]int64{}, "&$804;"
	if sold {
		wantAdena, wantStakes[4], wantOdd5 = 4000, 1000, "1.3"
	} else {
		w.assertChat(t, "confirm after the close", got, sysMsg(derby.MsgTicketsNotAvailable))
	}
	if adena := w.held(t, item.AdenaID); adena != wantAdena {
		t.Fatalf("sold %t: adena = %d, want %d", sold, adena, wantAdena)
	}
	if stakes := w.track.Stakes(); stakes != wantStakes {
		t.Fatalf("sold %t: stakes = %v, want %v", sold, stakes, wantStakes)
	}
	odds := []string{"Odd1", "&$804;", "Odd2", "&$804;", "Odd3", "&$804;", "Odd4", "&$804;", "Odd5", wantOdd5, "Odd6", "&$804;", "Odd7", "&$804;", "Odd8", "&$804;", "1race", "1"}
	w.assertPage(t, "odds", w.dialog(t, "ShowOdds"), derbyPage(t, w.manager, "30995-5.htm", append(runnerNames(), odds...)...))
}
