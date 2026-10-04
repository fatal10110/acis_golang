package olympiad

import (
	"context"
	"maps"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/rs/zerolog"
)

// recordingStore keeps every SaveNobles call and answers the rest with
// nothing.
type recordingStore struct {
	mu    sync.Mutex
	saves []map[int32]Noble
}

func (s *recordingStore) LoadCycle(context.Context) (int32, bool, error) { return 0, false, nil }
func (s *recordingStore) SaveCycle(context.Context, int32) error         { return nil }
func (s *recordingStore) LoadNobles(context.Context) (map[int32]Noble, error) {
	return map[int32]Noble{}, nil
}

func (s *recordingStore) SaveNobles(_ context.Context, nobles map[int32]Noble) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saves = append(s.saves, maps.Clone(nobles))
	return nil
}
func (s *recordingStore) DeleteNobles(context.Context) error  { return nil }
func (s *recordingStore) SnapshotMonth(context.Context) error { return nil }
func (s *recordingStore) SaveFight(context.Context, Fight) error {
	return nil
}

func (s *recordingStore) ClassLeaders(context.Context, int, int) ([]string, error) {
	return nil, nil
}

type silentAnnouncer struct{}

func (silentAnnouncer) Announce(Notice, int32) {}

// testNow is the clock every registration test reads.
var testNow = time.Date(2026, 10, 7, 20, 0, 0, 0, time.UTC)

// newRegistrationOlympiad returns an Olympiad in period, ending left after
// testNow, holding nobles, its writes made inline to the returned store.
func newRegistrationOlympiad(period Period, left time.Duration, nobles map[int32]Noble) (*Olympiad, *recordingStore) {
	store := &recordingStore{}
	o := New(DefaultConfig(), store, nil, silentAnnouncer{}, nil, sim.NewInline(testNow).NewQueue("olympiad"), zerolog.Nop())
	o.period, o.periodEnd = period, testNow.Add(left).UnixMilli()
	if nobles != nil {
		o.nobles = nobles
	}
	return o, store
}

// eligible is a noble every registration check lets through.
func eligible(id int32) Applicant {
	return Applicant{ObjectID: id, Name: "Noble", BaseClass: 88, Noble: true}
}

// TestRegisterChecks pins OlympiadManager.registerNoble and checkNoble:
// the window open with ten minutes or more left (600000 ms exactly is
// still open), then noble, base class, cursed weapon, weight, waiting
// lists (the non-classed one named first) and the starting record's
// points, each refusal in that order.
func TestRegisterChecks(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		period Period
		left   time.Duration
		edit   func(*Applicant)
		want   RegisterResult
	}{
		{"validation", Validation, time.Hour, nil, RegisterNotInProgress},
		{"validation beats every other check", Validation, time.Minute, func(a *Applicant) { a.Noble = false }, RegisterNotInProgress},
		{"window closing", Competition, 10*time.Minute - time.Millisecond, nil, RegisterClosing},
		{"window closing beats noble", Competition, time.Minute, func(a *Applicant) { a.Noble = false }, RegisterClosing},
		{"ten minutes left", Competition, 10 * time.Minute, nil, Registered},
		{"not noble", Competition, time.Hour, func(a *Applicant) { a.Noble, a.SubclassActive = false, true }, RegisterNotNoble},
		{"subclass", Competition, time.Hour, func(a *Applicant) { a.SubclassActive, a.CursedWeaponID = true, 8190 }, RegisterSubclass},
		{"cursed weapon", Competition, time.Hour, func(a *Applicant) { a.CursedWeaponID, a.Overweight = 8190, true }, RegisterCursedWeapon},
		{"overweight", Competition, time.Hour, func(a *Applicant) { a.Overweight = true }, RegisterOverweight},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			o, _ := newRegistrationOlympiad(tc.period, tc.left, nil)
			a := eligible(7)
			if tc.edit != nil {
				tc.edit(&a)
			}
			if got := o.Register(a, Classed); got != tc.want {
				t.Fatalf("Register = %v, want %v", got, tc.want)
			}
			_, hasRecord := o.Noble(7)
			if hasRecord != (tc.want == Registered) {
				t.Fatalf("record kept = %v after %v", hasRecord, tc.want)
			}
		})
	}
}

// TestRegisterWaitingLists pins the lists: a classed registration goes to
// the base class's list, a non-classed one to the shared list; a second
// registration of either kind is refused naming the list held, the
// non-classed one first; a new record starts with OlyStartPoints, the
// class and the name; and the waiting list counts the classes ever
// registered for, even emptied, and the non-classed registrations.
func TestRegisterWaitingLists(t *testing.T) {
	t.Parallel()
	o, _ := newRegistrationOlympiad(Competition, time.Hour, nil)
	if got := o.Register(eligible(1), Classed); got != Registered {
		t.Fatalf("classed Register = %v", got)
	}
	if n, ok := o.Noble(1); !ok || n != (Noble{ClassID: 88, Name: "Noble", Points: 18}) {
		t.Fatalf("new record = %+v, %v; want 18 points for class 88", n, ok)
	}
	for _, kind := range []GameType{Classed, NonClassed} {
		if got := o.Register(eligible(1), kind); got != RegisterAlreadyClassed {
			t.Fatalf("second Register(%v) = %v, want RegisterAlreadyClassed", kind, got)
		}
	}
	other := eligible(2)
	other.BaseClass = 93
	if got := o.Register(other, NonClassed); got != Registered {
		t.Fatalf("non-classed Register = %v", got)
	}
	for _, kind := range []GameType{Classed, NonClassed} {
		if got := o.Register(other, kind); got != RegisterAlreadyNonClassed {
			t.Fatalf("second Register(%v) = %v, want RegisterAlreadyNonClassed", kind, got)
		}
	}
	if !o.IsRegistered(1, 88) || o.IsRegistered(1, 93) || !o.IsRegistered(2, 93) || !o.IsRegisteredInComp(2, 93) {
		t.Fatal("IsRegistered does not follow the lists")
	}
	if classed, nonClassed := o.WaitingList(); classed != 1 || nonClassed != 1 {
		t.Fatalf("WaitingList = %d, %d; want 1, 1", classed, nonClassed)
	}
	o.RemoveDisconnectedCompetitor(1, 88)
	o.RemoveDisconnectedCompetitor(2, 93)
	if o.IsRegistered(1, 88) || o.IsRegistered(2, 93) {
		t.Fatal("RemoveDisconnectedCompetitor left a registration")
	}
	if classed, nonClassed := o.WaitingList(); classed != 1 || nonClassed != 0 {
		t.Fatalf("WaitingList after removals = %d, %d; want 1 (the emptied class), 0", classed, nonClassed)
	}
}

// TestRegisterWithoutPoints pins a noble whose record holds no points:
// refused, the record kept as it was, and on no list; a starting record of
// 0 points is kept and refused the same way.
func TestRegisterWithoutPoints(t *testing.T) {
	t.Parallel()
	o, _ := newRegistrationOlympiad(Competition, time.Hour, map[int32]Noble{1: {ClassID: 88, Points: 0, CompDone: 4}})
	if got := o.Register(eligible(1), Classed); got != RegisterNoPoints {
		t.Fatalf("Register = %v, want RegisterNoPoints", got)
	}
	if n, _ := o.Noble(1); n.CompDone != 4 || o.IsRegistered(1, 88) {
		t.Fatalf("record = %+v, registered %v", n, o.IsRegistered(1, 88))
	}

	o, _ = newRegistrationOlympiad(Competition, time.Hour, nil)
	o.cfg.StartPoints = 0
	if got := o.Register(eligible(2), NonClassed); got != RegisterNoPoints {
		t.Fatalf("Register with 0 starting points = %v", got)
	}
	if _, ok := o.Noble(2); !ok {
		t.Fatal("the starting record was not kept")
	}
}

// TestUnregister pins OlympiadManager.unRegisterNoble: window, noble and
// registration checks in that order, then the registration removed.
func TestUnregister(t *testing.T) {
	t.Parallel()
	o, _ := newRegistrationOlympiad(Competition, time.Hour, nil)
	if got := o.Unregister(1, 88, false); got != UnregisterNotNoble {
		t.Fatalf("non-noble Unregister = %v", got)
	}
	if got := o.Unregister(1, 88, true); got != UnregisterNotRegistered {
		t.Fatalf("unregistered Unregister = %v", got)
	}
	o.Register(eligible(1), Classed)
	o.Register(Applicant{ObjectID: 2, BaseClass: 90, Noble: true}, NonClassed)
	if got := o.Unregister(1, 88, true); got != Unregistered || o.IsRegistered(1, 88) {
		t.Fatalf("classed Unregister = %v", got)
	}
	if got := o.Unregister(2, 90, true); got != Unregistered || o.IsRegistered(2, 90) {
		t.Fatalf("non-classed Unregister = %v", got)
	}
	o.Register(eligible(1), Classed)
	o.period = Validation
	if got := o.Unregister(1, 88, false); got != UnregisterNotInProgress || !o.IsRegistered(1, 88) {
		t.Fatalf("Unregister in validation = %v", got)
	}
}

// TestPasses pins Olympiad.getNoblessePasses with the default rates
// (OlyGPPerPoint 1000, OlyHeroPoints 300): points capped at 1000 and none
// under 50, hero points added, the record left rewarded with no points and
// saved at once; nothing in the competition window, without a record, or a
// second time.
func TestPasses(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		points int
		hero   bool
		want   int
	}{
		{0, false, 0},
		{49, false, 0},
		{50, false, 50000},
		{1000, false, 1000000},
		{1500, false, 1000000},
		{30, true, 300000},
		{120, true, 420000},
	} {
		o, store := newRegistrationOlympiad(Validation, time.Hour, map[int32]Noble{1: {ClassID: 88, Points: tc.points, CompDone: 9}})
		if got := o.Passes(1, tc.hero); got != tc.want {
			t.Errorf("Passes(%d points, hero %v) = %d, want %d", tc.points, tc.hero, got, tc.want)
		}
		want := Noble{ClassID: 88, CompDone: 9, Rewarded: true}
		if n, _ := o.Noble(1); n != want {
			t.Errorf("record after the claim = %+v, want %+v", n, want)
		}
		if len(store.saves) != 1 || store.saves[0][1] != want || len(store.saves[0]) != 1 {
			t.Errorf("saves = %+v, want the claimed record alone", store.saves)
		}
		if got := o.Passes(1, tc.hero); got != 0 || len(store.saves) != 1 {
			t.Errorf("second claim = %d with %d saves, want 0 and no write", got, len(store.saves))
		}
	}

	o, store := newRegistrationOlympiad(Competition, time.Hour, map[int32]Noble{1: {Points: 100}})
	if got := o.Passes(1, true); got != 0 || len(store.saves) != 0 {
		t.Fatalf("claim in the window = %d, %d saves", got, len(store.saves))
	}
	if n, _ := o.Noble(1); n.Points != 100 || n.Rewarded {
		t.Fatalf("record after a refused claim = %+v", n)
	}
	o.period = Validation
	if got := o.Passes(2, true); got != 0 {
		t.Fatalf("claim without a record = %d", got)
	}
}

// TestPassesRace runs claims, registrations and saves of the records at
// once: only one claim pays, and the race detector finds no unguarded
// access.
func TestPassesRace(t *testing.T) {
	t.Parallel()
	o, _ := newRegistrationOlympiad(Validation, time.Hour, map[int32]Noble{1: {Points: 100}})
	var wg sync.WaitGroup
	paid := make(chan int, 8)
	for range 8 {
		wg.Go(func() {
			paid <- o.Passes(1, false)
			o.saveNobles()
			o.Register(eligible(3), Classed)
			_ = o.IsRegisteredInComp(3, 88)
		})
	}
	wg.Wait()
	close(paid)
	total := 0
	for p := range paid {
		total += p
	}
	if total != 100000 {
		t.Fatalf("paid %d in all, want one claim of 100000", total)
	}
}
