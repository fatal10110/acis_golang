package derby

import (
	"context"
	"testing"

	"github.com/rs/zerolog"
)

// fakeStore keeps the stored stakes in memory and counts the stakes saved.
type fakeStore struct {
	bets  []Bet
	saved []Bet
}

func (s *fakeStore) LoadHistory(context.Context) ([]History, error) { return nil, nil }
func (s *fakeStore) LoadBets(context.Context) ([]Bet, error)        { return s.bets, nil }
func (s *fakeStore) SaveBet(_ context.Context, b Bet) error {
	s.saved = append(s.saved, b)
	return nil
}
func (s *fakeStore) ClearBets(context.Context) error            { return nil }
func (s *fakeStore) SaveHistory(context.Context, History) error { return nil }

type fakeIDs struct{ next int32 }

func (f *fakeIDs) NextID() (int32, error) {
	f.next++
	return f.next, nil
}

// fakeRand keeps the runners in template order and draws 0.
type fakeRand struct{}

func (fakeRand) IntN(int) int                { return 0 }
func (fakeRand) Shuffle(int, func(int, int)) {}

// noPages has no page at all; every page answers with the missing notice.
type noPages struct{}

func (noPages) Get(string) (string, bool) { return "", false }

type noTickets struct{}

func (noTickets) Tickets() []Ticket         { return nil }
func (noTickets) Item(int32) (Ticket, bool) { return Ticket{}, false }

func testTemplates() []Template {
	var out []Template
	for id := FirstRunnerID; id <= LastRunnerID; id++ {
		out = append(out, Template{NpcID: id, Name: "Runner"})
	}
	return out
}

func newTestTrack(t *testing.T, store *fakeStore) *Track {
	t.Helper()
	track, err := New(context.Background(), store, testTemplates(), &fakeIDs{}, nil, fakeRand{}, zerolog.Nop())
	if err != nil {
		t.Fatal(err)
	}
	return track
}

// TestNewLeavesOutStakesOutsideTheEightLanes pins the Decision on stored
// stakes: a mdt_bets row for a lane outside 1 to 8 is left out, so the
// eight lanes keep their own stakes in their own slots.
func TestNewLeavesOutStakesOutsideTheEightLanes(t *testing.T) {
	track := newTestTrack(t, &fakeStore{bets: []Bet{{Lane: 1, Amount: 100}, {Lane: 9, Amount: 500}, {Lane: 0, Amount: 7}, {Lane: 8, Amount: 30}}})
	if got, want := track.Stakes(), [Lanes]int64{0: 100, 7: 30}; got != want {
		t.Fatalf("stakes = %v, want %v", got, want)
	}
}

// TestBuyTicketRefusesALanePastTheEighth pins the Decision on purchases: a
// ninth lane picked, with a price already picked, answers the purchase with
// nothing at all, and a stake placed on it changes no lane and stores
// nothing.
func TestBuyTicketRefusesALanePastTheEighth(t *testing.T) {
	store := &fakeStore{}
	track := newTestTrack(t, store)
	track.Tick()

	var picks Picks
	for _, step := range []string{"BuyTicket 1", "BuyTicket 13", "BuyTicket 9"} {
		if reply := track.Bypass(noPages{}, 30995, 1, &picks, noTickets{}, step); reply.Aborted {
			t.Fatalf("%s aborted", step)
		}
	}
	if picks != (Picks{Lane: 9, Price: 3}) {
		t.Fatalf("picks = %+v, want lane 9 at 1000 adena", picks)
	}
	if reply := track.Bypass(noPages{}, 30995, 1, &picks, noTickets{}, "BuyTicket 21"); reply != (Reply{Aborted: true}) {
		t.Fatalf("buying lane 9 = %+v, want aborted", reply)
	}

	for _, lane := range []int{0, 9, -1} {
		track.PlaceBet(lane, 500)
	}
	if got := track.Stakes(); got != ([Lanes]int64{}) {
		t.Fatalf("stakes after bets on lanes 0, 9 and -1 = %v, want none", got)
	}
	if len(store.saved) != 0 {
		t.Fatalf("stored %v for lanes outside 1 to 8, want nothing", store.saved)
	}

	// The eighth lane still sells.
	picks = Picks{Lane: 8, Price: 3}
	reply := track.Bypass(noPages{}, 30995, 1, &picks, noTickets{}, "BuyTicket 21")
	if reply.Buy == nil || *reply.Buy != (Purchase{Race: 1, Lane: 8, Price: 1000}) || !reply.Chat {
		t.Fatalf("buying lane 8 = %+v, want a 1000-adena ticket on lane 8 of race 1", reply)
	}
}

// TestCalculateOddsSharesSeventyPercent pins the odds: none on a lane with
// no stake, else 70% of every stake over the lane's own, never under 1.25.
func TestCalculateOddsSharesSeventyPercent(t *testing.T) {
	track := newTestTrack(t, &fakeStore{})
	track.bets = [Lanes]int64{1: 100, 4: 1000}
	track.calculateOddsLocked()
	want := []float64{0, 7.7, 0, 0, 1.25, 0, 0, 0}
	if len(track.odds) != len(want) {
		t.Fatalf("odds = %v, want %v", track.odds, want)
	}
	for i := range want {
		if diff := track.odds[i] - want[i]; diff > 1e-9 || diff < -1e-9 {
			t.Fatalf("odds = %v, want %v", track.odds, want)
		}
	}

	track.bets = [Lanes]int64{}
	track.calculateOddsLocked()
	for i, odd := range track.odds {
		if odd != 0 {
			t.Fatalf("odds with no stake = %v, lane %d not 0", track.odds, i+1)
		}
	}
}
