package clan

import (
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/clanhall"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/siege"
)

// Reference: ClanHallSiege.endSiege runs from the siege task and from a hall
// script's onMyDying (FlagWar, DevastatedCastle, FortressOfTheDead,
// FortressOfResistance), each guarded by isInSiege, then cancelSiegeTask.
// A dying hook that passed isInSiege just before the armed end set
// REGISTRATION_OPENED runs endSiege a second time: the reference races the
// pool thread against the game thread there. Here an end whose turn comes
// with the siege no longer fought does nothing.

// TestBanditStrongholdEndRunsOncePerSiege: the free hall's tryouts run with
// Besiegers and Holders. Holders' warrior and every warrior of no clan but
// the last fall early; the last, the deciding one, falls a second before
// the tryouts' 20 minutes run out, inside its dying hooks' 3 s delay. The
// armed end runs first: the match is drawn, the hall stays free and the
// next siege is armed. The dying hook then finds no siege fought, and the
// end its race would have run late, after its check passed, leaves the
// siege as it is: nobody is told anything, the hall stays free and the next
// siege date stands.
func TestBanditStrongholdEndRunsOncePerSiege(t *testing.T) {
	t.Parallel()
	besiegers, holders := strconv.Itoa(int(besiegersClanID)), strconv.Itoa(int(holdersClanID))
	w := bootFlagWar(t, flagWarSetup{seed: []string{
		"INSERT INTO clanhall_flagwar_attackers VALUES (35, 35423, 0, " + besiegers + ")",
		"INSERT INTO clanhall_flagwar_attackers VALUES (35, 35424, 35430, " + holders + ")",
		"INSERT INTO clanhall_flagwar_members SELECT 35, " + besiegers + ", obj_Id FROM characters WHERE char_name = 'Attacker'",
	}})
	if !w.siege.Instant() {
		t.Fatal("no script runs the Bandit Stronghold's siege")
	}
	w.srv.Advance(t, 0)
	w.srv.Advance(t, 300*time.Second)
	if !w.siege.InProgress() {
		t.Fatal("the tryouts did not begin")
	}
	timeOut := w.srv.Halls.Now() + (20 * time.Minute).Milliseconds()

	w.kill(t, w.warriors(ally3ID)[0])
	var clanless []*npc.Hostile
	for _, h := range w.warriors(ally2ID) {
		if h.Instance.ClanID != besiegersClanID {
			clanless = append(clanless, h)
		}
	}
	if len(clanless) < 2 {
		t.Fatalf("%d warriors of no clan stand, want at least 2", len(clanless))
	}
	for _, h := range clanless[:len(clanless)-1] {
		w.kill(t, h)
	}
	if !w.siege.InProgress() {
		t.Fatal("the siege ended with a warrior of no clan standing")
	}
	drainFrames(t, w.c)

	// The deciding warrior falls a second before the time-out; its dying
	// hooks run 2 s after the armed end.
	w.srv.Advance(t, time.Duration(timeOut-w.srv.Halls.Now()-1000)*time.Millisecond)
	w.kill(t, clanless[len(clanless)-1])
	got := flat(sysMsgParams(t, drainFrames(t, w.c)))
	draw, over := slices.Index(got, strconv.Itoa(msgMatchDraw)+" 35"), slices.Index(got, strconv.Itoa(msgSiegeFinished)+" 35")
	if draw < 0 || over < draw || slices.Contains(got, strconv.Itoa(msgClanDefeated)+" Besiegers 35") {
		t.Fatalf("the time-out's notices = %v, want %d then %d", got, msgMatchDraw, msgSiegeFinished)
	}
	if !w.siege.Registering() {
		t.Fatal("the armed end left the registrations closed")
	}
	next := w.siege.Date()

	// The end the dying hook would run late, its check passed before the
	// armed end's.
	if !w.siege.End() {
		t.Fatal("no script runs the Bandit Stronghold's siege")
	}
	w.srv.Advance(t, 0)
	if got := sysMsgParams(t, drainFrames(t, w.c)); len(got) != 0 {
		t.Fatalf("the late end told %v", got)
	}
	if owner := w.hallOwner(t); owner != 0 {
		t.Fatalf("hall owner after the late end = %#x, want the hall free", owner)
	}
	if !w.siege.Registering() || w.siege.Date() != next {
		t.Fatalf("after the late end: registering %v, next siege %d; want registering, %d", w.siege.Registering(), w.siege.Date(), next)
	}
}

// TestHallSiegeLateEndDropsItsWin: Devastated Castle's siege, Besiegers its
// only attacker and the script's winner, runs its hour out with no mission
// accomplished: a draw. A script's dying hook racing the time-out records
// the win and ends the siege late: nothing happens, the script's NPCs are
// not taken away twice and the next date stands. The next siege, run out
// the same way, is a draw too: the late win does not carry over.
func TestHallSiegeLateEndDropsItsWin(t *testing.T) {
	t.Parallel()
	w := bootHallSiege(t, hallSiegeSetup{})
	w.script.winner.Store(besiegersClanID)
	runOut := func(step string) []string {
		t.Helper()
		w.siege.AddAttacker(besiegersClanID)
		if !w.siege.Instant() {
			t.Fatalf("%s: no siege to start", step)
		}
		w.srv.Advance(t, 0)
		if !w.siege.InProgress() {
			t.Fatalf("%s: the siege did not start", step)
		}
		drainFrames(t, w.c)
		w.srv.Advance(t, time.Hour)
		if !w.siege.Registering() {
			t.Fatalf("%s: the siege did not end", step)
		}
		return flat(sysMsgParams(t, drainFrames(t, w.c)))
	}
	wantDraw := func(step string, got []string) {
		t.Helper()
		if !slices.Contains(got, strconv.Itoa(int(siege.MsgSiegeDraw))+" 34") ||
			slices.Contains(got, strconv.Itoa(int(clanhall.MsgClanDefeated))+" Besiegers 34") {
			t.Fatalf("%s: notices = %v, want a draw", step, got)
		}
		if owner, _, _ := w.hallRow(t); owner != 0 {
			t.Fatalf("%s: hall owner = %#x, want the hall free", step, owner)
		}
	}

	wantDraw("the first time-out", runOut("the first siege"))
	next := w.siege.Date()

	w.siege.SetMissionAccomplished(true)
	if !w.siege.End() {
		t.Fatal("no script runs Devastated Castle's siege")
	}
	w.srv.Advance(t, 0)
	if got := sysMsgParams(t, drainFrames(t, w.c)); len(got) != 0 {
		t.Fatalf("the late end told %v", got)
	}
	if n := w.script.unspawned.Load(); n != 1 {
		t.Fatalf("the siege's NPCs were taken away %d times, want once", n)
	}
	if _, _, endDate := w.hallRow(t); w.siege.Date() != next || endDate != next {
		t.Fatalf("next siege after the late end = %d, stored %d; want %d", w.siege.Date(), endDate, next)
	}

	wantDraw("the second time-out", runOut("the second siege"))
}
