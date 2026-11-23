package olympiad

import (
	"context"
	"database/sql"
	"errors"
	"maps"
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/hero"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// electionAttempts is how many attempts an election makes before the
// running era keeps its heroes.
const electionAttempts = 3

var errInjectedRead = errors.New("injected hero read fault")

// faultyHeroStore fails the next failures reads of failClass's hero to be
// and counts every read of it.
type faultyHeroStore struct {
	hero.Store
	mu        sync.Mutex
	failClass int
	failures  int
	reads     int
}

func (s *faultyHeroStore) BestNoble(ctx context.Context, classID, minMatches int) (int32, string, bool, error) {
	s.mu.Lock()
	if classID == s.failClass {
		s.reads++
		if s.failures > 0 {
			s.failures--
			s.mu.Unlock()
			return 0, "", false, errInjectedRead
		}
	}
	s.mu.Unlock()
	return s.Store.BestNoble(ctx, classID, minMatches)
}

func (s *faultyHeroStore) set(failures int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failures, s.reads = failures, 0
}

func (s *faultyHeroStore) readCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reads
}

// electionState is what an election changes: the heroes rows and the
// rows of the hero items the seed placed.
type electionState struct {
	heroes                            map[int32]heroRow
	alphaBlade, alphaAdena, gmCirclet int
}

func readElectionState(t *testing.T, srv *gameservertest.Server) electionState {
	t.Helper()
	return electionState{
		heroes:     heroRows(t, srv),
		alphaBlade: itemCount(t, srv, alpha, infinityBlade),
		gmCirclet:  itemCount(t, srv, gm, heroCirclet),
		alphaAdena: itemCount(t, srv, alpha, 57),
	}
}

func (s electionState) equal(o electionState) bool {
	return maps.Equal(s.heroes, o.heroes) && s.alphaBlade == o.alphaBlade && s.gmCirclet == o.gmCirclet && s.alphaAdena == o.alphaAdena
}

// electedState is electionSeed's election, stored.
func electedState(login int32) electionState {
	return electionState{
		heroes: map[int32]heroRow{
			login:   {classID: 88, count: 1, active: true},
			bravo:   {classID: 88, count: 1, played: true},
			echo:    {classID: 93, count: 3, played: true},
			foxtrot: {classID: 94, count: 1},
			golf:    {classID: 91, count: 1, played: true},
		},
		alphaAdena: 1,
		gmCirclet:  1,
	}
}

// bootElection boots electionSeed's ending era, its heroes kept in store.
func bootElection(t *testing.T, store *faultyHeroStore) (*gameservertest.Server, int32) {
	t.Helper()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Login", 40, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithHeroStore(func(st hero.Store) hero.Store {
			store.Store = st
			return store
		}),
		gameservertest.WithOlympiadSeed(func(db *sql.DB) {
			for _, stmt := range electionSeed() {
				if _, err := db.Exec(stmt); err != nil {
					t.Fatalf("seed %q: %v", stmt, err)
				}
			}
		}))
	return srv, srv.SoleObjectID(t)
}

func runElection(t *testing.T, srv *gameservertest.Server) {
	t.Helper()
	srv.Olympiad.SelectHeroes()
	srv.Settle(t)
	srv.FlushPersistence(t)
	srv.Settle(t)
}

// wantElected checks that electionSeed's election is stored and in
// memory: Login and Foxtrot left the running era, Bravo, Echo and Golf
// are its inactive heroes.
func wantElected(t *testing.T, srv *gameservertest.Server, login int32) {
	t.Helper()
	want := electedState(login)
	if got := readElectionState(t, srv); !got.equal(want) {
		t.Fatalf("after the election = %+v, want %+v", got, want)
	}
	for id, inactive := range map[int32]bool{bravo: true, echo: true, golf: true, login: false, foxtrot: false} {
		if got := srv.Heroes.IsInactive(id); got != inactive {
			t.Errorf("IsInactive(%d) = %v, want %v", id, got, inactive)
		}
	}
	if srv.Heroes.IsActive(login) {
		t.Error("the outgoing hero is still active")
	}
}

// wantKept checks that a failed election left the ending era as it was:
// the stored heroes, their played flags and the hero items, and Login
// active and Foxtrot inactive in memory.
func wantKept(t *testing.T, srv *gameservertest.Server, login int32, before electionState) {
	t.Helper()
	if got := readElectionState(t, srv); !got.equal(before) {
		t.Fatalf("after a failed election = %+v, want %+v kept", got, before)
	}
	if !srv.Heroes.IsActive(login) || !srv.Heroes.IsInactive(foxtrot) {
		t.Fatal("a failed election changed the running era's heroes in memory")
	}
	if srv.Heroes.IsInactive(bravo) || srv.Heroes.IsInactive(golf) {
		t.Fatal("a failed election elected heroes in memory")
	}
}

// TestHeroElectionReadFailure pins an election whose read of a class's
// hero to be fails (#4697): a read failing on every attempt leaves the
// ending era's heroes, stored and in memory, and every hero item as they
// were, and a later election completes; a read failing once is read again
// and the election completes.
func TestHeroElectionReadFailure(t *testing.T) {
	t.Parallel()
	t.Run("every attempt fails", func(t *testing.T) {
		t.Parallel()
		store := &faultyHeroStore{failClass: 90}
		srv, login := bootElection(t, store)
		before := readElectionState(t, srv)
		store.set(electionAttempts)
		runElection(t, srv)
		if got := store.readCount(); got != electionAttempts {
			t.Fatalf("class 90 reads = %d, want %d attempts", got, electionAttempts)
		}
		wantKept(t, srv, login, before)

		store.set(0)
		runElection(t, srv)
		wantElected(t, srv, login)
	})
	t.Run("one attempt fails", func(t *testing.T) {
		t.Parallel()
		store := &faultyHeroStore{failClass: 90}
		srv, login := bootElection(t, store)
		store.set(1)
		runElection(t, srv)
		if got := store.readCount(); got != 2 {
			t.Fatalf("class 90 reads = %d, want 2", got)
		}
		wantElected(t, srv, login)
	})
}

// TestHeroElectionWriteFailure pins an election whose heroes the database
// refuses to store (#4697): the end of the era and the hero item deletes
// roll back with it, so the stored heroes, their played flags, the hero
// items and the heroes in memory are as they were; once the database
// takes the heroes again, a later election completes.
func TestHeroElectionWriteFailure(t *testing.T) {
	t.Parallel()
	store := &faultyHeroStore{failClass: -1}
	srv, login := bootElection(t, store)
	before := readElectionState(t, srv)
	ctx := context.Background()
	drop := func() {
		if _, err := srv.DB.ExecContext(ctx, "DROP TRIGGER IF EXISTS hero_election_fault"); err != nil {
			t.Errorf("drop fault trigger: %v", err)
		}
	}
	t.Cleanup(drop)
	if _, err := srv.DB.ExecContext(ctx, `CREATE TRIGGER hero_election_fault BEFORE INSERT ON heroes FOR EACH ROW
		SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'injected hero save fault'`); err != nil {
		t.Fatalf("create fault trigger: %v", err)
	}
	runElection(t, srv)
	wantKept(t, srv, login, before)

	drop()
	runElection(t, srv)
	wantElected(t, srv, login)
}
