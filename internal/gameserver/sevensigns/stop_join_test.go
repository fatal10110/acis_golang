package sevensigns

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
)

// observedTimer is an armed timer whose Stop closes stopped, so a test
// learns when State.Stop has latched.
type observedTimer struct {
	once    sync.Once
	stopped chan struct{}
}

func (t *observedTimer) Stop() bool {
	t.once.Do(func() { close(t.stopped) })
	return true
}

// heldWrites keeps each job handed to it for the test to run.
type heldWrites struct{ jobs []func() }

func (w *heldWrites) Enqueue(_ int32, job func()) bool {
	w.jobs = append(w.jobs, job)
	return true
}

// heldOnline records the players-online calls and keeps the dungeon
// sweep's continuation for the test to release.
type heldOnline struct {
	*journal
	expelled func()
}

func (o *heldOnline) GiveStrifeSkills(then func())   { then() }
func (o *heldOnline) RemoveStrifeSkills(then func()) { then() }
func (o *heldOnline) ExpelFromDungeons(then func()) {
	o.entries = append(o.entries, "dungeons swept")
	o.expelled = then
}

// stopHarness is a state whose period change runs on an inline queue the
// test drives and saves through writes the test runs.
type stopHarness struct {
	state  *State
	store  *recordingStore
	online *heldOnline
	queue  *sim.Inline
	writes *heldWrites
	timer  *observedTimer
	fire   func()
	armed  int
}

func newStopHarness(t *testing.T) *stopHarness {
	t.Helper()
	start := at(2026, time.August, 24, 17, 0, time.UTC)
	h := &stopHarness{
		store:  &recordingStore{row: StatusRow{Cycle: 4, Period: Recruiting, LastSave: start}, found: true},
		queue:  sim.NewInline(start),
		writes: &heldWrites{},
		timer:  &observedTimer{stopped: make(chan struct{})},
	}
	h.online = &heldOnline{journal: &journal{store: h.store}}
	h.state = NewState(h.store, h.online, zerolog.Nop(), func() time.Time { return start }, h.queue.NewQueue("sevensigns"), h.writes,
		func(_ time.Duration, fn func()) Timer {
			h.armed++
			h.fire = fn
			return h.timer
		})
	if err := h.state.Restore(context.Background()); err != nil {
		t.Fatal(err)
	}
	h.state.SetOnline(h.online)
	h.state.Start()
	return h
}

// stop calls Stop on its own goroutine and returns once it has latched,
// with a channel closed when Stop returns.
func (h *stopHarness) stop(t *testing.T) <-chan struct{} {
	t.Helper()
	returned := make(chan struct{})
	go func() {
		h.state.Stop()
		close(returned)
	}()
	<-h.timer.stopped
	select {
	case <-returned:
		t.Fatal("Stop returned with the period change still under way")
	default:
	}
	return returned
}

// runSave runs the one save the period change handed to the writer.
func (h *stopHarness) runSave(t *testing.T) {
	t.Helper()
	if len(h.writes.jobs) != 1 {
		t.Fatalf("saves handed to the writer = %d, want 1", len(h.writes.jobs))
	}
	job := h.writes.jobs[0]
	h.writes.jobs = nil
	job()
}

// assertQuietAfterStop checks that nothing of the period change runs once
// Stop returned: no task left to post, no save, no new timer.
func (h *stopHarness) assertQuietAfterStop(t *testing.T, saves int) {
	t.Helper()
	q := h.state.queue
	accepted := q.Accepted()
	h.queue.Run()
	if q.Accepted() != accepted || len(h.writes.jobs) != 0 || len(h.store.saves) != saves || h.armed != 1 {
		t.Fatalf("after Stop: queue tasks %d -> %d, writer jobs %d, status saves %d (want %d), timers armed %d (want 1)",
			accepted, q.Accepted(), len(h.writes.jobs), len(h.store.saves), saves, h.armed)
	}
}

// A Stop arriving while the period change waits on its save waits for
// it: the new period is still written, then the change ends without
// sweeping the dungeons, showing the sky or re-arming, and nothing of it
// posts or writes once Stop returns.
func TestStopJoinsPeriodChangeAwaitingItsSave(t *testing.T) {
	h := newStopHarness(t)
	h.fire()
	h.queue.Run()

	returned := h.stop(t)
	h.runSave(t)
	if len(h.store.saves) != 1 || h.store.saves[0].Period != Competition {
		t.Fatalf("status saves = %+v, want the competition's", h.store.saves)
	}
	select {
	case <-returned:
		t.Fatal("Stop returned before the step after the save ran")
	default:
	}
	h.queue.Run()
	<-returned

	if slices.Contains(h.online.entries, "dungeons swept") || slices.Contains(h.online.entries, fmt.Sprintf("notice %d", NoticeSky)) {
		t.Fatalf("entries = %q, want no sweep and no sky after Stop", h.online.entries)
	}
	h.assertQuietAfterStop(t, 1)
}

// A Stop arriving while the dungeon sweep is still on the players' queues
// waits for it, then the change ends without showing the sky or re-arming.
func TestStopJoinsPeriodChangeSweepingDungeons(t *testing.T) {
	h := newStopHarness(t)
	h.fire()
	h.queue.Run()
	h.runSave(t)
	h.queue.Run()
	if h.online.expelled == nil {
		t.Fatalf("entries = %q, want the dungeon sweep under way", h.online.entries)
	}

	returned := h.stop(t)
	h.online.expelled()
	h.queue.Run()
	<-returned

	if slices.Contains(h.online.entries, fmt.Sprintf("notice %d", NoticeSky)) {
		t.Fatalf("entries = %q, want no sky after Stop", h.online.entries)
	}
	h.assertQuietAfterStop(t, 1)
}

// Without Stop the same steps run through: saved, swept, the sky shown,
// and the next change armed.
func TestPeriodChangeStepsRunThroughOnItsQueue(t *testing.T) {
	h := newStopHarness(t)
	h.fire()
	h.queue.Run()
	h.runSave(t)
	h.queue.Run()
	h.online.expelled()
	h.queue.Run()

	want := []string{"notice 0", "notice 1", "dungeons swept", fmt.Sprintf("notice %d", NoticeSky)}
	if !slices.Equal(h.online.entries, want) || h.armed != 2 {
		t.Fatalf("entries = %q, timers armed %d; want %q and the next change armed", h.online.entries, h.armed, want)
	}
}
