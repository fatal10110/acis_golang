package clan

import (
	"slices"
	"testing"
	"time"
)

// Owner ruling on #4693: arming a hall-siege step stops the step armed
// before it, so a start forced ahead of its date leaves no stale close of
// the registrations behind.

// TestHallSiegeInstantStartDropsTheScheduledClose: Devastated Castle boots
// with its siege a week ahead and its registrations' close armed an hour
// before. The siege is started at once (//ch siege start), Besiegers win
// it and own the hall, and the next date is set two weeks on. When the old
// date's close comes, nothing happens: the registrations stay open, the
// owner keeps the hall and joins no attackers. At the new date the siege
// starts once, its NPCs spawned once more.
func TestHallSiegeInstantStartDropsTheScheduledClose(t *testing.T) {
	t.Parallel()
	w := bootHallSiege(t, hallSiegeSetup{})
	w.script.winner.Store(besiegersClanID)
	oldDate := w.siege.Date()

	w.siege.AddAttacker(besiegersClanID)
	if !w.siege.Instant() {
		t.Fatal("no script runs Devastated Castle's siege")
	}
	w.srv.Advance(t, 0)
	if !w.siege.InProgress() {
		t.Fatal("the instant start did not start the siege")
	}
	w.siege.SetMissionAccomplished(true)
	w.srv.Advance(t, time.Hour)
	if !w.siege.Registering() {
		t.Fatal("the siege did not end after its hour")
	}
	if owner, _, _ := w.hallRow(t); owner != int64(besiegersClanID) {
		t.Fatalf("hall owner after the win = %#x, want Besiegers", owner)
	}
	newDate := w.siege.Date()
	if newDate <= oldDate {
		t.Fatalf("next siege %d, want past the old date %d", newDate, oldDate)
	}

	// Past the old date's close, and the old date itself.
	w.srv.Advance(t, time.Duration(oldDate-w.srv.Halls.Now()+1000)*time.Millisecond)
	if !w.siege.Registering() || w.siege.WaitingBattle() || w.siege.InProgress() {
		t.Fatalf("at the old date: status %v, want the registrations open", w.siege.Status())
	}
	if owner, _, endDate := w.hallRow(t); owner != int64(besiegersClanID) || endDate != newDate {
		t.Fatalf("at the old date: hall owner %#x, stored date %d; want Besiegers, %d", owner, endDate, newDate)
	}
	if got := w.storedAttackers(t, devastatedCastle); len(got) != 0 {
		t.Fatalf("at the old date: stored attackers %v, want none", got)
	}
	if n := w.script.spawned.Load(); n != 1 {
		t.Fatalf("NPC spawns by the old date = %d, want 1", n)
	}

	// The new date's close takes the owner in as attacker; the siege starts
	// once.
	w.srv.Advance(t, time.Duration(newDate-w.srv.Halls.Now()+1000)*time.Millisecond)
	if !w.siege.InProgress() {
		t.Fatalf("at the new date: status %v, want the siege fought", w.siege.Status())
	}
	if got := w.siege.Attackers(); !slices.Equal(got, []int32{besiegersClanID}) {
		t.Fatalf("at the new date: attackers %v, want Besiegers", got)
	}
	if n := w.script.spawned.Load(); n != 2 {
		t.Fatalf("NPC spawns after the second start = %d, want 2 (one per siege)", n)
	}
}
