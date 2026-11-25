package npcs

import (
	"testing"
	"time"
)

// TestChamberlainSiegeTimeSetPastTheWindowMovesNothing keeps the siege hour
// choice page open past the window (#5114): a day before the siege, and
// once the date is closed to change, siege_time_set is refused with the page
// siege_change answers with, and the stored siege date stays as it is.
func TestChamberlainSiegeTimeSetPastTheWindowMovesNothing(t *testing.T) {
	t.Parallel()
	w := bootChamberlain(t, commandSetup{})
	c, _ := w.srv.Castles.Get(taxCastleID)
	setDate := func(date time.Time, over bool) int64 {
		t.Helper()
		c.SetSiegeDate(date.UnixMilli())
		c.SetTimeRegistrationOver(over)
		c.SaveSiegeInfo()
		return date.UnixMilli()
	}
	checkStored := func(when string, date int64, over string) {
		t.Helper()
		if _, _, _, got, gotOver := w.castleRow(t); got != date || gotOver != over {
			t.Fatalf("%s: stored siege date %d over %q, want %d %s", when, got, gotOver, date, over)
		}
	}

	open := time.Date(2030, 6, 9, 18, 0, 0, 0, time.Local)
	setDate(open, false)
	assertAnswer(t, w.run(t, "siege_change"), pageThenFinal, w.f, w.filledFile(t, "siegetime3.htm"))
	assertAnswer(t, w.run(t, "siege_time_set 0"), pageThenFinal, w.f, w.filledFile(t, "siegetime6.htm"))

	// The last day before the siege: the registrations have ended.
	soon := setDate(time.Now().Add(time.Hour).Truncate(time.Second), false)
	for _, command := range []string{"siege_time_set 1 4", "siege_time_set 1 8", "siege_time_set 0"} {
		assertAnswer(t, w.run(t, command), pageThenFinal, w.f, w.filledFile(t, "siegetime1.htm"))
		checkStored(command+" a day before the siege", soon, "false")
	}

	// A date already closed to change.
	closed := setDate(open, true)
	for _, command := range []string{"siege_time_set 1 4", "siege_time_set 1 8", "siege_time_set 0"} {
		assertAnswer(t, w.run(t, command), pageThenFinal, w.f, w.filledFile(t, "siegetime2.htm"))
		checkStored(command+" on a closed date", closed, "true")
	}
}

// TestChamberlainSiegeHourChoiceIsNotKept: an hour picked for one siege
// date does not carry over (#5114). Once the date opens again, as for the
// next cycle or a new lord, "Yes" on siegetime3 opens the hour choice
// instead of moving the siege to the hour picked before, and picking the
// default hour leaves the date as it is.
func TestChamberlainSiegeHourChoiceIsNotKept(t *testing.T) {
	t.Parallel()
	w := bootChamberlain(t, commandSetup{})
	c, _ := w.srv.Castles.Get(taxCastleID)

	first := time.Date(2030, 6, 9, 18, 0, 0, 0, time.Local)
	c.SetSiegeDate(first.UnixMilli())
	c.SetTimeRegistrationOver(false)
	assertAnswer(t, w.run(t, "siege_time_set 1 4"), pageThenFinal, w.f, w.filledFile(t, "siegetime8.htm"))
	moved := time.Date(2030, 6, 9, 16, 0, 0, 0, time.Local).UnixMilli()
	if _, _, _, date, over := w.castleRow(t); date != moved || over != "true" {
		t.Fatalf("stored siege date %d over %q, want %d true", date, over, moved)
	}

	next := time.Date(2030, 6, 23, 18, 0, 0, 0, time.Local).UnixMilli()
	c.SetSiegeDate(next)
	c.SetTimeRegistrationOver(false)
	c.SaveSiegeInfo()
	assertAnswer(t, w.run(t, "siege_change"), pageThenFinal, w.f, w.filledFile(t, "siegetime3.htm"))
	assertAnswer(t, w.run(t, "siege_time_set 0"), pageThenFinal, w.f, w.filledFile(t, "siegetime6.htm"))
	assertAnswer(t, w.run(t, "siege_time_set 1 6"), pageThenFinal, w.f, w.filledFile(t, "siegetime6.htm"))
	if _, _, _, date, over := w.castleRow(t); date != next || over != "false" {
		t.Fatalf("next date: stored siege date %d over %q, want %d false", date, over, next)
	}
	assertAnswer(t, w.run(t, "siege_time_set 1 8"), pageThenFinal, w.f, w.filledFile(t, "siegetime8.htm"))
	if _, _, _, date, over := w.castleRow(t); date != next+(2*time.Hour).Milliseconds() || over != "true" {
		t.Fatalf("next date moved: stored siege date %d over %q, want %d true", date, over, next+(2*time.Hour).Milliseconds())
	}
}
