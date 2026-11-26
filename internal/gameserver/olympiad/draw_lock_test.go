package olympiad

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
)

// reentrantArena is a draftArena whose Draft calls back into the Olympiad,
// as a draft reaching a player's registration would.
type reentrantArena struct {
	draftArena
	o *Olympiad
	// seen records what each draft found for the player drafted:
	// registering again, leaving its list, and whether it counts as
	// registered in the competition.
	seen []draftView
}

type draftView struct {
	register   RegisterResult
	unregister UnregisterResult
	inComp     bool
}

func (a *reentrantArena) Draft(objectID int32, stadium int) {
	a.draftArena.Draft(objectID, stadium)
	a.seen = append(a.seen, draftView{
		register:   a.o.Register(eligible(objectID), NonClassed),
		unregister: a.o.Unregister(objectID, true),
		inComp:     a.o.IsRegisteredInComp(objectID),
	})
}

// The draw drafts its competitors after it releases the Olympiad's lock,
// and a competitor drawn counts as in its match from the draw on: a draft
// that registers or unregisters the player finds it competing, and does
// not deadlock.
func TestDrawDraftsOutsideTheLockAndHoldsTheDrawnInTheMatch(t *testing.T) {
	t.Parallel()
	o, _ := newRegistrationOlympiad(Competition, time.Hour, nil)
	o.cfg.NonClassedParticipants = 2
	arena := &reentrantArena{o: o}
	// The stadiums' queues never run: a drafted match stays attached.
	o.BindStadiums(2, arena, sim.NewInline(testNow).NewQueue, func(int) int { return 0 })
	for id := int32(1); id <= 2; id++ {
		if got := o.Register(eligible(id), NonClassed); got != Registered {
			t.Fatalf("Register(%d) = %v", id, got)
		}
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		o.games.draw()
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("draw did not return: Olympiad.mu is held across Arena.Draft")
	}

	if want := [][2]int32{{1, 0}, {2, 0}}; !slices.Equal(arena.drafts, want) {
		t.Fatalf("drafts = %v, want %v", arena.drafts, want)
	}
	// Drawn, the player is off its list: leaving finds no registration.
	want := draftView{register: RegisterInMatch, unregister: UnregisterNotRegistered, inComp: true}
	for i, got := range arena.seen {
		if got != want {
			t.Fatalf("draft %d saw %+v, want %+v", i, got, want)
		}
	}
	if o.IsRegistered(1) || o.IsRegistered(2) {
		t.Fatal("a drafted competitor is back on a waiting list")
	}
	if o.mu.Lock(); len(o.games.drawn) != 0 {
		t.Fatalf("drawn = %v after the draw, want none", o.games.drawn)
	}
	o.mu.Unlock()
}
