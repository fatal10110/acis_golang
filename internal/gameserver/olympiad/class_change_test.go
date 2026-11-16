package olympiad

import (
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
)

// TestRegistrationFollowsTheNobleNotItsClass pins the owner-approved fix of
// #5116: a noble is on at most one waiting list whatever class it names,
// and leaving finds its registration under any class.
func TestRegistrationFollowsTheNobleNotItsClass(t *testing.T) {
	t.Parallel()
	o, _ := newRegistrationOlympiad(Competition, time.Hour, nil)
	if got := o.Register(eligible(1), Classed); got != Registered {
		t.Fatalf("Register = %v", got)
	}
	moved := eligible(1)
	moved.BaseClass = 93
	for _, kind := range []GameType{Classed, NonClassed} {
		if got := o.Register(moved, kind); got != RegisterAlreadyClassed {
			t.Fatalf("Register(%v) under another class = %v, want RegisterAlreadyClassed", kind, got)
		}
	}
	if !o.IsRegistered(1) || !o.IsRegisteredInComp(1) {
		t.Fatal("the registration under class 88 is not seen")
	}
	if got := o.Unregister(1, true); got != Unregistered || o.IsRegistered(1) {
		t.Fatalf("Unregister = %v, registered %v; want Unregistered, off every list", got, o.IsRegistered(1))
	}
	if got := o.Register(moved, NonClassed); got != Registered {
		t.Fatalf("Register once off the list = %v", got)
	}
	o.RemoveDisconnectedCompetitor(1, -1)
	if o.IsRegistered(1) {
		t.Fatal("RemoveDisconnectedCompetitor left the registration")
	}
}

// TestChangeClass pins the owner-approved fix of #5116 for a base class
// that moves: the noble leaves its waiting list and its record counts for
// the new class, that record's row saved at once; a noble on no list is
// told so, and one without a record gets none.
func TestChangeClass(t *testing.T) {
	t.Parallel()
	o, store := newRegistrationOlympiad(Competition, time.Hour, nil)
	if got := o.Register(eligible(1), Classed); got != Registered {
		t.Fatalf("Register = %v", got)
	}
	if !o.ChangeClass(1, 93) {
		t.Fatal("ChangeClass reports no registration left")
	}
	want := Noble{ClassID: 93, Name: "Noble", Points: 18}
	if n, _ := o.Noble(1); n != want || o.IsRegistered(1) {
		t.Fatalf("record = %+v, registered %v; want %+v off every list", n, o.IsRegistered(1), want)
	}
	if len(store.saves) != 1 || !maps.Equal(store.saves[0], map[int32]Noble{1: want}) {
		t.Fatalf("saves = %+v, want the moved record alone", store.saves)
	}
	if classed, _ := o.WaitingList(); classed != 1 {
		t.Fatalf("classed waiting list = %d, want 1 (class 88 keeps its empty list)", classed)
	}
	moved := eligible(1)
	moved.BaseClass = 93
	if got := o.Register(moved, Classed); got != Registered {
		t.Fatalf("Register under the new class = %v", got)
	}

	if o.ChangeClass(2, 93) {
		t.Fatal("a noble on no list left one")
	}
	if _, ok := o.Noble(2); ok || len(store.saves) != 1 {
		t.Fatalf("a noble without a record got one, saves %d", len(store.saves))
	}
}

// draftArena records the drafts and finds every player online; nothing
// else of Arena is used while its stadiums' queues never run.
type draftArena struct {
	Arena
	drafts [][2]int32
}

func (a *draftArena) Competitor(objectID int32) (Competitor, bool) {
	return Competitor{Name: "Noble", BaseClass: 88}, true
}

func (a *draftArena) Draft(objectID int32, stadium int) {
	a.drafts = append(a.drafts, [2]int32{objectID, int32(stadium)})
}

// TestDrawSkipsACompetitorInAMatch pins the owner-approved guard of #5116:
// a player already competing, found on a waiting list, is never drafted
// into a second match; it is dropped from the list like one offline.
func TestDrawSkipsACompetitorInAMatch(t *testing.T) {
	t.Parallel()
	o, _ := newRegistrationOlympiad(Competition, time.Hour, nil)
	o.cfg.NonClassedParticipants = 2
	arena := &draftArena{}
	// The stadiums' queues never run: a drafted match stays attached.
	o.BindStadiums(2, arena, sim.NewInline(testNow).NewQueue, func(int) int { return 0 })
	for id := int32(1); id <= 3; id++ {
		if id == 3 {
			o.games.draw()
		}
		if got := o.Register(eligible(id), NonClassed); got != Registered {
			t.Fatalf("Register(%d) = %v", id, got)
		}
	}
	// Registration refuses a competitor; put 1 back beside 3 as a second
	// list once let it be.
	o.mu.Lock()
	o.nonClassed = append(o.nonClassed, 1)
	o.mu.Unlock()
	o.games.draw()
	if want := [][2]int32{{1, 0}, {2, 0}}; !slices.Equal(arena.drafts, want) {
		t.Fatalf("drafts = %v, want %v", arena.drafts, want)
	}
	if o.mu.Lock(); !slices.Equal(o.nonClassed, []int32{3}) {
		t.Fatalf("non-classed list = %v, want [3]", o.nonClassed)
	}
	o.mu.Unlock()
}
