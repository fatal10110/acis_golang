package admin

import (
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/clanhall"
	"github.com/fatal10110/acis_golang/internal/gameserver/siege"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// Reference: AdminClanHall.useAdminCommand ("siege" start|end), with
// ClanHallSiege.instantSiege (prepareSiege(0)) and endSiege. The shipped
// Fortress of Resistance (21) is a siegable hall whose siege starts with
// no attacker.

// TestAdminClanHallSiegeStartEnd drives Fortress of Resistance's siege,
// run by a stand-in script, through //ch siege. end on a siege not being
// fought says so. start closes the registrations (everyone, the GM
// included, told REGISTRATION_TERM_FOR_S1_ENDED naming the hall) before
// the page, and the siege starts at once on the halls' queue, the script
// spawning its NPCs. start on the siege being fought says it is already
// besieged. end ends it before the page: a draw (S1_SIEGE_WAS_DRAWN) then
// SIEGE_OF_S1_FINISHED, the registrations open again and the script takes
// its NPCs away.
func TestAdminClanHallSiegeStartEnd(t *testing.T) {
	t.Parallel()
	var spawned, unspawned atomic.Int32
	script := clanhall.SiegeScript{
		SpawnNPCs:   func() { spawned.Add(1) },
		UnspawnNPCs: func() { unspawned.Add(1) },
	}
	next := time.Now().Add(7 * 24 * time.Hour).UnixMilli()
	w := bootClanHallAdminWith(t, false,
		[]string{"INSERT INTO clanhall (id, endDate) VALUES (" + strconv.Itoa(chFortress) + ", " + strconv.FormatInt(next, 10) + ")"},
		gameservertest.WithHallSiegeScript(chFortress, script),
	)
	if !w.srv.DrivesClock() {
		t.Skip("the hall siege steps run on the driven clock")
	}
	s, ok := w.srv.Halls.Siege(chFortress)
	if !ok {
		t.Fatal("Fortress of Resistance has no siege")
	}
	fortressPage := func(page string) {
		t.Helper()
		if !strings.Contains(page, "Fortress of Resistance") {
			t.Fatalf("page =\n%s\nwant Fortress of Resistance's", page)
		}
	}

	fortressPage(wantTextThenPage(t, w.ch(t, "siege end 21"), "This ClanHall isn't currently besieged."))

	frames := w.ch(t, "siege start 21")
	if len(frames) != 2 || systemMessageID(frames[0]) != siege.MsgRegistrationTermEnded {
		t.Fatalf("siege start 21 frames = %x, want REGISTRATION_TERM_FOR_S1_ENDED then the page", testsupport.FrameOpcodes(frames))
	}
	fortressPage(htmlBody(t, frames[1]))
	w.srv.Advance(t, 0)
	if !s.InProgress() || spawned.Load() != 1 {
		t.Fatalf("after siege start: in progress %v, spawns %d; want under way with one spawn", s.InProgress(), spawned.Load())
	}

	fortressPage(wantTextThenPage(t, w.ch(t, "siege start 21"), "This ClanHall is already besieged."))
	if !s.InProgress() || spawned.Load() != 1 {
		t.Fatalf("a second start moved the siege: in progress %v, spawns %d", s.InProgress(), spawned.Load())
	}

	frames = w.ch(t, "siege end 21")
	if len(frames) != 3 || systemMessageID(frames[0]) != siege.MsgSiegeDraw || systemMessageID(frames[1]) != clanhall.MsgSiegeFinished {
		t.Fatalf("siege end 21 frames = %x, want S1_SIEGE_WAS_DRAWN, SIEGE_OF_S1_FINISHED then the page", testsupport.FrameOpcodes(frames))
	}
	fortressPage(htmlBody(t, frames[2]))
	if s.Status() != siege.StatusRegistrationOpened || unspawned.Load() != 1 {
		t.Fatalf("after siege end: status %v, unspawns %d; want the registrations open and one unspawn", s.Status(), unspawned.Load())
	}

	fortressPage(wantTextThenPage(t, w.ch(t, "siege end 21"), "This ClanHall isn't currently besieged."))
}
