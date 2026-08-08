package effect

import (
	"testing"
	"time"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

func TestSeedRestoreSchedulesFromPersistedCountAndElapsedTime(t *testing.T) {
	e := &Effect{Template: modelskill.EffectTemplate{Count: 5, Time: 10}}
	e.seedRestore(3, 4) // 3 ticks left, 4s elapsed since the last tick at logout
	e.startSchedule(time.Unix(1000, 0))

	if got := e.Remaining(); got != 3 {
		t.Fatalf("Remaining() = %d, want 3 (persisted count, below template count 5)", got)
	}

	fixedNow := time.Unix(1000, 0)
	if run, _ := e.claimAction(fixedNow.Add(5 * time.Second)); run {
		t.Fatal("claimAction fired before the seeded delay (10-4=6s) elapsed")
	}

	// claimAction above only peeked before the delay elapsed and didn't
	// mutate anything (nextAction is still 6s out), so the same schedule can
	// be claimed again at the 6s mark without reseeding.
	if run, remove := e.claimAction(fixedNow.Add(6 * time.Second)); !run || remove {
		t.Fatalf("claimAction at the seeded delay = run %v remove %v, want run=true remove=false (2 ticks remain)", run, remove)
	}
	if got := e.Remaining(); got != 2 {
		t.Fatalf("Remaining() after first tick = %d, want 2", got)
	}
}

func TestSeedRestoreClampsCountToTemplateCountAndElapsedToPeriod(t *testing.T) {
	e := &Effect{Template: modelskill.EffectTemplate{Count: 2, Time: 10}}
	e.seedRestore(99, 999) // persisted values exceeding the template's own count/period
	fixedNow := time.Unix(2000, 0)
	e.startSchedule(fixedNow)

	if got := e.Remaining(); got != 2 {
		t.Fatalf("Remaining() = %d, want clamped to template count 2", got)
	}
	if run, _ := e.claimAction(fixedNow); !run {
		t.Fatal("claimAction did not fire immediately when the elapsed time exceeded the period")
	}
}

func TestSeedRestoreNonPeriodicEffectNeverClaims(t *testing.T) {
	e := &Effect{Template: modelskill.EffectTemplate{Count: 1}}
	e.seedRestore(1, 0)
	e.startSchedule(time.Now())

	if run, remove := e.claimAction(time.Now().Add(time.Hour)); run || remove {
		t.Fatalf("claimAction on a non-periodic restored effect = run %v remove %v, want both false", run, remove)
	}
}

// TestApplyRestoredDeliversOnStartToLiveEffectList is the regression case
// for the reported gap: a restored effect used to sit inert in
// Persistence's save registry and never reached the live effect list, so
// its OnStart hook (icons, stat application, ExRegenMax, ...) never fired
// on relog. ApplyRestored is what Persistence.ReplayEffects now calls to
// replay it through List.Add like a live cast would.
// TestSaveStateIsTheInverseOfSeedRestore proves SaveState and seedRestore
// round-trip: an effect scheduled with a persisted count/elapsed reports
// that same count/elapsed back out once time has actually advanced by the
// elapsed amount, the way a logout mid-period should re-persist the buff's
// remaining state rather than resetting or losing it.
func TestSaveStateIsTheInverseOfSeedRestore(t *testing.T) {
	e := &Effect{Template: modelskill.EffectTemplate{Count: 5, Time: 10}}
	e.seedRestore(3, 4) // 3 ticks left, 4s elapsed since the last tick at logout
	start := time.Unix(1000, 0)
	e.startSchedule(start)

	count, elapsed := e.SaveState(start)
	if count != 3 || elapsed != 4 {
		t.Fatalf("SaveState() at the seeded instant = (%d, %d), want (3, 4)", count, elapsed)
	}

	count, elapsed = e.SaveState(start.Add(2 * time.Second))
	if count != 3 || elapsed != 6 {
		t.Fatalf("SaveState() 2s later = (%d, %d), want (3, 6)", count, elapsed)
	}
}

func TestSaveStateOnAFreshEffectReportsNoElapsedTime(t *testing.T) {
	e := &Effect{Template: modelskill.EffectTemplate{Count: 2, Time: 30}}
	now := time.Unix(5000, 0)
	e.startSchedule(now)

	count, elapsed := e.SaveState(now)
	if count != 2 || elapsed != 0 {
		t.Fatalf("SaveState() right after a fresh cast = (%d, %d), want (2, 0)", count, elapsed)
	}
}

// TestSaveStateFloorsSubSecondElapsedInsteadOfRoundingUp guards against
// truncating the remaining-until-next-tick duration to whole seconds before
// subtracting it from the period: doing so floors "remaining" (rounding it
// down) and so rounds the derived elapsed time up, over-reporting by up to a
// full second right after a fresh cast. SaveState must floor the elapsed
// duration itself instead, matching the whole-seconds-so-far semantic
// startScheduleFromRestoreLocked's delay computation expects on restore.
func TestSaveStateFloorsSubSecondElapsedInsteadOfRoundingUp(t *testing.T) {
	e := &Effect{Template: modelskill.EffectTemplate{Count: 1, Time: 30}}
	start := time.Unix(7000, 0)
	e.startSchedule(start)

	if _, elapsed := e.SaveState(start.Add(time.Millisecond)); elapsed != 0 {
		t.Fatalf("SaveState() 1ms after cast = elapsed %d, want 0 (not rounded up to 1)", elapsed)
	}
	if _, elapsed := e.SaveState(start.Add(29*time.Second + 999*time.Millisecond)); elapsed != 29 {
		t.Fatalf("SaveState() just before the tick = elapsed %d, want 29 (not rounded up to 30, which would restore as an immediate tick)", elapsed)
	}
}

func TestSaveStateOnANonPeriodicEffectReportsNoElapsedTime(t *testing.T) {
	e := &Effect{Template: modelskill.EffectTemplate{Count: 1}}
	now := time.Unix(6000, 0)
	e.startSchedule(now)

	count, elapsed := e.SaveState(now.Add(time.Hour))
	if count != 1 || elapsed != 0 {
		t.Fatalf("SaveState() on a non-periodic effect = (%d, %d), want (1, 0)", count, elapsed)
	}
}

func TestApplyRestoredDeliversOnStartToLiveEffectList(t *testing.T) {
	target := &fakeChargesTarget{}
	events := []string{}
	list := NewList(eventOwner{events: &events})
	meta := Skill{ID: 7, Level: 3}
	templates := []modelskill.EffectTemplate{{Name: "IncreaseCharges", Value: 2, Count: 5}}

	ApplyRestored(list, target, target, meta, templates, 5, 0)

	if target.charges != 2 || target.max != 5 {
		t.Fatalf("ApplyRestored charges/max = %d/%d, want 2/5 (OnStart delivered on restore, like a live cast)", target.charges, target.max)
	}
	active := list.All()
	if len(active) != 1 || !active[0].InUse() {
		t.Fatal("ApplyRestored effect never became active in the live list")
	}
	if got := active[0].Remaining(); got != 5 {
		t.Fatalf("Remaining() = %d, want 5 (persisted count == template count, no clamp)", got)
	}
}

func TestApplyRestoredSkipsUnsupportedTemplatesWithoutFailingTheRest(t *testing.T) {
	target := &fakeChargesTarget{}
	events := []string{}
	list := NewList(eventOwner{events: &events})
	meta := Skill{ID: 8}
	templates := []modelskill.EffectTemplate{
		{Name: "not-a-real-effect"},
		{Name: "IncreaseCharges", Value: 1, Count: 3},
	}

	ApplyRestored(list, target, target, meta, templates, 3, 0)

	if len(list.All()) != 1 {
		t.Fatalf("ApplyRestored added %d effects, want 1 (unsupported template skipped)", len(list.All()))
	}
}
