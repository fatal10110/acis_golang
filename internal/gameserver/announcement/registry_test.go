package announcement

import (
	"errors"
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

// An automatic announcement with a delay under a second or a negative
// limit is refused (#5126): nothing is held, saved or scheduled. One that
// is not automatic keeps no schedule values, so its values are not checked.
func TestAddRefusesFloodingSchedules(t *testing.T) {
	store := &fakeStore{}
	r, clock, rec := newTestRegistry(t, store)
	for _, c := range []struct {
		a    admin.Announcement
		want error
	}{
		{auto("zero", 5, 0, 0), ErrDelay},
		{auto("zero limited", 5, 0, 3), ErrDelay},
		{auto("negative", 5, -10, 0), ErrDelay},
		{auto("negative limit", 5, 10, -2), ErrLimit},
		{auto("", 5, 10, 0), ErrEmptyMessage},
		{admin.Announcement{}, ErrEmptyMessage},
	} {
		if err := r.Add(c.a); !errors.Is(err, c.want) {
			t.Errorf("Add(%+v) = %v, want %v", c.a, err, c.want)
		}
	}
	clock.Advance(time.Hour)
	if len(rec.fires) != 0 || len(r.List()) != 0 || len(store.saved) != 0 {
		t.Fatalf("after refusals: delivered %v, held %v, saved %v, want nothing", rec.fires, r.List(), store.saved)
	}
	if at, ok := clock.NextTimer(); ok {
		t.Fatalf("a timer is armed for %v, want none", at)
	}

	login := admin.Announcement{Message: "login", Auto: false, Delay: -1, Limit: -1}
	if err := r.Add(login); err != nil {
		t.Fatalf("Add(%+v) = %v, want nil", login, err)
	}
	if got, want := r.List(), []Indexed{{Index: 0, Announcement: admin.Announcement{Message: "login"}}}; !slices.Equal(got, want) {
		t.Fatalf("List() = %+v, want %+v", got, want)
	}
}

// Load skips a stored automatic announcement with a delay under a second
// or a negative limit (#5126), so no schedule without pause starts; the
// rest are indexed from 0 in file order.
func TestLoadSkipsFloodingSchedules(t *testing.T) {
	store := &fakeStore{list: []admin.Announcement{
		auto("zero", 0, 0, 0),
		auto("zero limited", 0, 0, 5),
		auto("kept", 5, 10, 0),
		auto("negative limit", 0, 10, -1),
		{Message: "login"},
	}}
	r, clock, rec := newTestRegistry(t, store)
	if err := r.Load(); err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	want := []Indexed{{Index: 0, Announcement: auto("kept", 5, 10, 0)}, {Index: 1, Announcement: admin.Announcement{Message: "login"}}}
	if got := r.List(); !slices.Equal(got, want) {
		t.Fatalf("List() = %+v, want %+v", got, want)
	}
	clock.Advance(20 * time.Second)
	if len(rec.fires) != 2 {
		t.Fatalf("delivered %v, want only kept at 5 s and 15 s", rec.fires)
	}
	assertTimes(t, rec, "kept", 5, 15)
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

// A negative initial delay counts as none.
func TestNegativeInitialDelayCountsAsZero(t *testing.T) {
	r, clock, rec := newTestRegistry(t, nil)
	r.Add(auto("unlimited", -5, 10, 0))
	r.Add(auto("limited", -3, 7, 3))

	clock.Advance(0)
	assertTimes(t, rec, "unlimited", 0)
	assertTimes(t, rec, "limited", 0)

	clock.Advance(25 * time.Second)
	assertTimes(t, rec, "unlimited", 0, 10, 20)
	assertTimes(t, rec, "limited", 0, 7, 14)
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

// An announcement added after a deletion takes an index never handed out
// before, even when the highest one was deleted, so it replaces nothing
// (#4707); the deleted automatic announcement stops at once and Stop leaves
// no timer armed.
func TestIndexNeverReusedAfterDelete(t *testing.T) {
	store := &fakeStore{}
	r, clock, rec := newTestRegistry(t, store)
	r.Add(admin.Announcement{Message: "login"})
	r.Add(auto("deleted", 10, 10, 0))
	r.Add(auto("kept", 10, 10, 0))
	r.Add(auto("top", 10, 10, 0))
	clock.Advance(10 * time.Second)
	if !r.Delete(1) || !r.Delete(3) {
		t.Fatal("Delete(1), Delete(3) = false, want true")
	}
	r.Add(auto("added", 5, 20, 0))

	want := []Indexed{
		{Index: 0, Announcement: admin.Announcement{Message: "login"}},
		{Index: 2, Announcement: auto("kept", 10, 10, 0)},
		{Index: 4, Announcement: auto("added", 5, 20, 0)},
	}
	if got := r.List(); !slices.Equal(got, want) {
		t.Fatalf("List() = %+v, want %+v", got, want)
	}
	r.Save()
	wantSaved := []admin.Announcement{{Message: "login"}, auto("kept", 10, 10, 0), auto("added", 5, 20, 0)}
	if got := store.saved[len(store.saved)-1]; !slices.Equal(got, wantSaved) {
		t.Fatalf("last save = %+v, want %+v", got, wantSaved)
	}

	clock.Advance(30 * time.Second) // to 40
	assertTimes(t, rec, "deleted", 10)
	assertTimes(t, rec, "top", 10)
	assertTimes(t, rec, "kept", 10, 20, 30, 40)
	assertTimes(t, rec, "added", 15, 35)

	r.Stop()
	clock.Advance(time.Hour)
	assertTimes(t, rec, "deleted", 10)
	assertTimes(t, rec, "top", 10)
	assertTimes(t, rec, "kept", 10, 20, 30, 40)
	assertTimes(t, rec, "added", 15, 35)
	if at, ok := clock.NextTimer(); ok {
		t.Fatalf("a timer is armed for %v after Stop, want none", at)
	}

	// A stopped registry starts no schedule again.
	r.Add(auto("late", 0, 10, 0))
	r.RestartAuto()
	clock.Advance(time.Hour)
	assertTimes(t, rec, "late")
}

// Loading again indexes the announcements from 0 in file order.
func TestLoadRestartsIndexes(t *testing.T) {
	store := &fakeStore{}
	r, _, _ := newTestRegistry(t, store)
	r.Add(admin.Announcement{Message: "first"})
	r.Add(admin.Announcement{Message: "second"})
	r.Delete(0)
	r.Save()
	store.list = store.saved[len(store.saved)-1]
	if err := r.Load(); err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	r.Add(admin.Announcement{Message: "third"})
	want := []Indexed{{Index: 0, Announcement: admin.Announcement{Message: "second"}}, {Index: 1, Announcement: admin.Announcement{Message: "third"}}}
	if got := r.List(); !slices.Equal(got, want) {
		t.Fatalf("List() = %+v, want %+v", got, want)
	}
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
func TestClampInt32ClampsOutOfRange(t *testing.T) {
	for _, c := range []struct {
		in   int
		want int32
	}{{-2, -2}, {math.MaxInt32, math.MaxInt32}, {math.MaxInt32 + 1, math.MaxInt32}, {math.MinInt32 - 1, math.MinInt32}} {
		if got := clampInt32(c.in); got != c.want {
			t.Errorf("clampInt32(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}
