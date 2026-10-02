package announcement

import (
	"math"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/admin"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/rs/zerolog"
)

// fakeStore serves list on Load and records every Save.
type fakeStore struct {
	list  []admin.Announcement
	saved [][]admin.Announcement
}

func (s *fakeStore) Load() ([]admin.Announcement, error) { return slices.Clone(s.list), nil }

func (s *fakeStore) Save(list []admin.Announcement) error {
	s.saved = append(s.saved, slices.Clone(list))
	return nil
}

// fire is one delivered announcement: its message and when it came, as an
// offset from the clock's start.
type fire struct {
	msg string
	at  time.Duration
}

// recorder records every announcement delivered, with the virtual time.
type recorder struct {
	clock *sim.Inline
	start time.Time
	fires []fire
}

func (r *recorder) Announce(message string, _ bool) {
	r.fires = append(r.fires, fire{msg: message, at: r.clock.Now().Sub(r.start)})
}

// times returns the offsets, in seconds, at which msg was delivered.
func (r *recorder) times(msg string) []int {
	var out []int
	for _, f := range r.fires {
		if f.msg == msg {
			out = append(out, int(f.at/time.Second))
		}
	}
	return out
}

func newTestRegistry(t *testing.T, store Store) (*Registry, *sim.Inline, *recorder) {
	t.Helper()
	start := time.Unix(1_700_000_000, 0)
	clock := sim.NewInline(start)
	rec := &recorder{clock: clock, start: start}
	r := NewRegistry(store, rec, zerolog.Nop(), clock.NewQueue("announcements"))
	t.Cleanup(r.Stop)
	return r, clock, rec
}

func auto(msg string, initial, delay, limit int) admin.Announcement {
	return admin.Announcement{Message: msg, Auto: true, InitialDelay: initial, Delay: delay, Limit: limit}
}

func assertTimes(t *testing.T, rec *recorder, msg string, want ...int) {
	t.Helper()
	if got := rec.times(msg); !slices.Equal(got, want) {
		t.Fatalf("%q delivered at %v s, want %v s", msg, got, want)
	}
}

// An unlimited announcement whose delay is not positive starts nothing, not
// even its first announcement: the reference's fixed-rate schedule refuses
// a period that is not positive.
func TestUnlimitedWithoutPositiveDelayStartsNothing(t *testing.T) {
	r, clock, rec := newTestRegistry(t, nil)
	r.Add(auto("zero", 5, 0, 0))
	r.Add(auto("negative", 5, -10, 0))

	clock.Advance(time.Hour)
	if len(rec.fires) != 0 {
		t.Fatalf("delivered %v, want nothing", rec.fires)
	}
	if at, ok := clock.NextTimer(); ok {
		t.Fatalf("a timer is armed for %v, want none", at)
	}
}

// An unlimited announcement comes after its initial delay, then once per
// delay for as long as it is held.
func TestUnlimitedRepeatsAtFixedRate(t *testing.T) {
	r, clock, rec := newTestRegistry(t, nil)
	r.Add(auto("forever", 5, 10, 0))

	clock.Advance(4 * time.Second)
	assertTimes(t, rec, "forever")
	clock.Advance(41 * time.Second)
	assertTimes(t, rec, "forever", 5, 15, 25, 35, 45)
}

// A limited announcement comes that many times, a delay apart, then stops.
func TestLimitedStopsAfterItsCount(t *testing.T) {
	r, clock, rec := newTestRegistry(t, nil)
	r.Add(auto("thrice", 5, 10, 3))

	clock.Advance(time.Hour)
	assertTimes(t, rec, "thrice", 5, 15, 25)
	if at, ok := clock.NextTimer(); ok {
		t.Fatalf("a timer is armed for %v after the last announcement, want none", at)
	}
}

// A negative limit counts down away from zero, so the announcement repeats
// past the limit's absolute value, indefinitely in practice.
func TestNegativeLimitRepeatsIndefinitely(t *testing.T) {
	r, clock, rec := newTestRegistry(t, nil)
	r.Add(auto("negative", 5, 10, -2))

	clock.Advance(95 * time.Second)
	assertTimes(t, rec, "negative", 5, 15, 25, 35, 45, 55, 65, 75, 85, 95)
}

// A negative initial delay or delay counts as none.
func TestNegativeDelaysCountAsZero(t *testing.T) {
	r, clock, rec := newTestRegistry(t, nil)
	r.Add(auto("unlimited", -5, 10, 0))
	r.Add(auto("limited", -3, -7, 3))

	clock.Advance(0)
	assertTimes(t, rec, "unlimited", 0)
	assertTimes(t, rec, "limited", 0, 0, 0)

	clock.Advance(25 * time.Second)
	assertTimes(t, rec, "unlimited", 0, 10, 20)
	assertTimes(t, rec, "limited", 0, 0, 0)
}

// Restarting the automatic announcements starts each schedule over from its
// initial delay; the timer of the earlier run delivers nothing more.
func TestRestartAutoStartsSchedulesOver(t *testing.T) {
	r, clock, rec := newTestRegistry(t, nil)
	r.Add(auto("forever", 5, 10, 0))
	r.Add(auto("twice", 5, 10, 2))

	clock.Advance(7 * time.Second)
	r.RestartAuto()
	clock.Advance(30 * time.Second) // to 37
	assertTimes(t, rec, "forever", 5, 12, 22, 32)
	assertTimes(t, rec, "twice", 5, 12, 22)
}

// An announcement added after a deletion takes the index equal to the
// number held, replacing the announcement still at that index. A replaced
// automatic announcement leaves the file and the list but keeps repeating;
// Stop ends it.
func TestReplacedAutomaticKeepsRepeatingUntilStop(t *testing.T) {
	store := &fakeStore{}
	r, clock, rec := newTestRegistry(t, store)
	r.Add(admin.Announcement{Message: "login"})
	r.Add(auto("replaced", 10, 10, 0))
	if !r.Delete(0) {
		t.Fatal("Delete(0) = false, want true")
	}
	r.Add(auto("replacing", 5, 20, 0))

	want := []Indexed{{Index: 1, Announcement: auto("replacing", 5, 20, 0)}}
	if got := r.List(); !slices.Equal(got, want) {
		t.Fatalf("List() = %+v, want %+v", got, want)
	}
	if got := store.saved[len(store.saved)-1]; !slices.Equal(got, []admin.Announcement{auto("replacing", 5, 20, 0)}) {
		t.Fatalf("last save = %+v, want only the replacing announcement", got)
	}

	clock.Advance(30 * time.Second)
	assertTimes(t, rec, "replaced", 10, 20, 30)
	assertTimes(t, rec, "replacing", 5, 25)

	r.Stop()
	clock.Advance(time.Hour)
	assertTimes(t, rec, "replaced", 10, 20, 30)
	assertTimes(t, rec, "replacing", 5, 25)
	if at, ok := clock.NextTimer(); ok {
		t.Fatalf("a timer is armed for %v after Stop, want none", at)
	}

	// A stopped registry starts no schedule again.
	r.Add(auto("late", 0, 10, 0))
	r.RestartAuto()
	clock.Advance(time.Hour)
	assertTimes(t, rec, "late")
}

// Loading again stops the schedules of the announcements it drops before
// starting the ones read anew.
func TestLoadAgainStopsDroppedSchedules(t *testing.T) {
	store := &fakeStore{list: []admin.Announcement{auto("old", 5, 10, 0), auto("old limited", 5, 10, 5)}}
	r, clock, rec := newTestRegistry(t, store)
	if err := r.Load(); err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	clock.Advance(15 * time.Second)
	assertTimes(t, rec, "old", 5, 15)
	assertTimes(t, rec, "old limited", 5, 15)

	store.list = []admin.Announcement{auto("new", 5, 10, 0)}
	if err := r.Load(); err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	clock.Advance(time.Minute) // to 75
	assertTimes(t, rec, "old", 5, 15)
	assertTimes(t, rec, "old limited", 5, 15)
	assertTimes(t, rec, "new", 20, 30, 40, 50, 60, 70)
	if got := r.List(); len(got) != 1 || got[0].Message != "new" {
		t.Fatalf("List() = %+v, want only the announcement read anew", got)
	}
	if len(store.saved) != 0 {
		t.Fatalf("Load rewrote the file %d times, want none", len(store.saved))
	}
}

// A limit out of the 32-bit range, which no caller passes today, clamps to
// the nearest bound instead of wrapping to the other sign.
func TestJavaIntClampsOutOfRange(t *testing.T) {
	for _, c := range []struct {
		in   int
		want int32
	}{{-2, -2}, {math.MaxInt32, math.MaxInt32}, {math.MaxInt32 + 1, math.MaxInt32}, {math.MinInt32 - 1, math.MinInt32}} {
		if got := javaInt(c.in); got != c.want {
			t.Errorf("javaInt(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}
