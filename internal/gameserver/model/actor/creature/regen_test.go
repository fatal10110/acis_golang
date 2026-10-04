package creature

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
)

// regenFixture is a creature whose shortfall the test sets, on a virtual
// clock that starts at the Unix epoch.
type regenFixture struct {
	loop  *sim.Inline
	q     *sim.Queue
	short bool
	regen Regen
}

func newRegenFixture() *regenFixture {
	loop := sim.NewInline(time.Unix(0, 0))
	return &regenFixture{loop: loop, q: loop.NewQueue("regen")}
}

func (f *regenFixture) settle() {
	f.regen.Settle(f.q, func() bool { return f.short })
}

// at moves the virtual clock to elapsed since the start and returns it.
func (f *regenFixture) at(elapsed time.Duration) time.Time {
	f.loop.Advance(elapsed - f.loop.Now().Sub(time.Unix(0, 0)))
	return f.loop.Now()
}

// claim reports whether the tick due at elapsed is claimed.
func (f *regenFixture) claim(elapsed time.Duration) bool {
	return f.regen.Claim(f.at(elapsed))
}

// TestRegenFirstTickOnePeriodAfterDrop pins CreatureStatus.
// startHpMpRegeneration's scheduleAtFixedRate(period, period): the first
// tick is due exactly one period after the first drop, then every period.
func TestRegenFirstTickOnePeriodAfterDrop(t *testing.T) {
	t.Parallel()
	f := newRegenFixture()
	f.at(1100 * time.Millisecond)
	f.short = true
	f.settle()

	if f.regen.Due(f.at(4099*time.Millisecond)) || f.claim(4099*time.Millisecond) {
		t.Fatal("tick due before one period after the drop")
	}
	if !f.regen.Due(f.at(4100*time.Millisecond)) || !f.claim(4100*time.Millisecond) {
		t.Fatal("tick not due one period after the drop")
	}
	if f.claim(4100 * time.Millisecond) {
		t.Fatal("one deadline claimed twice")
	}
	if f.claim(7099 * time.Millisecond) {
		t.Fatal("second tick due early")
	}
	if !f.claim(7100 * time.Millisecond) {
		t.Fatal("second tick not due one period after the first")
	}
}

// TestRegenLaterDropKeepsPhase pins startHpMpRegeneration's no-op while the
// task runs: another drop neither delays nor resets the next tick.
func TestRegenLaterDropKeepsPhase(t *testing.T) {
	t.Parallel()
	f := newRegenFixture()
	f.short = true
	f.settle()
	f.at(2900 * time.Millisecond)
	f.settle() // a hit just before the tick
	if !f.claim(3 * time.Second) {
		t.Fatal("a later drop moved the tick")
	}
}

// TestRegenRestartsWithFreshPhase pins stopHpMpRegeneration at full: once
// nothing is short the phase stops, and the next drop ticks one full period
// after itself, not on the old phase.
func TestRegenRestartsWithFreshPhase(t *testing.T) {
	t.Parallel()
	f := newRegenFixture()
	f.short = true
	f.settle()
	if !f.claim(3 * time.Second) {
		t.Fatal("first tick not due")
	}
	f.short = false // the tick filled it
	f.settle()
	if f.regen.Active() {
		t.Fatal("phase still armed at full")
	}

	f.at(5500 * time.Millisecond)
	f.short = true // hit again
	f.settle()
	if f.claim(6 * time.Second) {
		t.Fatal("the new phase ticked on the old grid")
	}
	if f.claim(8499 * time.Millisecond) {
		t.Fatal("the new phase ticked early")
	}
	if !f.claim(8500 * time.Millisecond) {
		t.Fatal("the new phase did not tick one period after the drop")
	}
}

// TestRegenClaimSkipsMissedDeadlines covers a sweep that stalled across
// several periods: one tick runs, then the phase resumes on its grid
// instead of running the missed ticks back to back.
func TestRegenClaimSkipsMissedDeadlines(t *testing.T) {
	t.Parallel()
	f := newRegenFixture()
	f.short = true
	f.settle()
	if !f.claim(10 * time.Second) {
		t.Fatal("overdue tick not claimed")
	}
	if f.claim(10*time.Second) || f.claim(11999*time.Millisecond) {
		t.Fatal("missed deadlines ran back to back")
	}
	if !f.claim(12 * time.Second) {
		t.Fatal("the phase left its 3s grid")
	}
}

// TestRegenIdleWithoutQueue leaves a creature that has no queue yet idle.
func TestRegenIdleWithoutQueue(t *testing.T) {
	t.Parallel()
	var r Regen
	r.Settle(nil, func() bool { return true })
	if r.Active() {
		t.Fatal("phase armed without a queue")
	}
}
