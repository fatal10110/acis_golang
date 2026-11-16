package festival

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
)

// A run's party registers its score once (owner-approved divergence,
// #5112): registering again with more offerings would otherwise win the
// cabal the festival's worth each time.

var dawnGuide = Guide{FestivalID: 0, Cabal: sevensigns.Dawn}

// newScoreHarness has party wait to register Dawn's festival 0, whose best
// scores of the cycle are dawnBest and 0 for Dusk.
func newScoreHarness(t *testing.T, dawnBest int) *scheduleHarness {
	t.Helper()
	h := newScheduleHarness(t)
	h.store.scores = []Score{
		{FestivalID: 0, Cabal: sevensigns.Dawn, Cycle: 1, Score: dawnBest},
		{FestivalID: 0, Cabal: sevensigns.Dusk, Cycle: 1},
	}
	if err := h.m.Restore(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	h.m.mu.Lock()
	h.m.previous[side(sevensigns.Dawn)][0] = party
	h.m.mu.Unlock()
	return h
}

// partyLeader is party's leader, its offerings taken by take.
func partyLeader(take func() (count int, held, taken bool)) GuideTalker {
	return GuideTalker{
		ObjectID:      party[0].ObjectID,
		Name:          party[0].Name,
		Party:         &GuideParty{Leader: party[0].ObjectID, Members: party},
		TakeOfferings: take,
	}
}

func (h *scheduleHarness) registerScore(t GuideTalker) GuideReply {
	return h.m.Command(dawnGuide, t, t.ObjectID, 3, "Festival 3")
}

func TestScoreRegistersOncePerRun(t *testing.T) {
	h := newScoreHarness(t, 0)
	var calls int
	leader := partyLeader(func() (int, bool, bool) {
		calls++
		return calls, true, true
	})
	if got := h.registerScore(leader); got.Page != "festival_3c.htm" || got.Contribution != 5 {
		t.Fatalf("first registration = page %q contribution %d, want festival_3c.htm 5", got.Page, got.Contribution)
	}
	for range 4 {
		if got := h.registerScore(leader); got.Page != "festival_3b.htm" || got.Contribution != 0 {
			t.Fatalf("registration again = page %q contribution %d, want festival_3b.htm 0", got.Page, got.Contribution)
		}
	}
	if calls != 1 {
		t.Fatalf("offerings taken %d times, want 1", calls)
	}
	if got := h.calendar.festivalScore[side(sevensigns.Dawn)].Load(); got != int64(MaxScores[0]) {
		t.Fatalf("Dawn festival score = %d, want %d", got, MaxScores[0])
	}
	best, _ := h.m.HighestScore(sevensigns.Dawn, 0)
	if best.Score != 5 || best.Members != "Ann,Bob,Cid" {
		t.Fatalf("Dawn best = %d by %q, want 5 by Ann,Bob,Cid", best.Score, best.Members)
	}
}

// A score short of the cabal's best (3d) is the run's registration too.
func TestScoreShortOfTheBestIsTheRunsRegistration(t *testing.T) {
	h := newScoreHarness(t, 50)
	if got := h.registerScore(partyLeader(func() (int, bool, bool) { return 1, true, true })); got.Page != "festival_3d.htm" {
		t.Fatalf("registration = page %q, want festival_3d.htm", got.Page)
	}
	if got := h.registerScore(partyLeader(func() (int, bool, bool) { return 20, true, true })); got.Page != "festival_3b.htm" {
		t.Fatalf("registration again = page %q, want festival_3b.htm", got.Page)
	}
	if got := h.calendar.festivalScore[side(sevensigns.Dawn)].Load(); got != 0 {
		t.Fatalf("Dawn festival score = %d, want 0", got)
	}
	if best, _ := h.m.HighestScore(sevensigns.Dawn, 0); best.Score != 50 {
		t.Fatalf("Dawn best = %d, want 50", best.Score)
	}
}

// Registrations that register nothing leave the party waiting.
func TestScoreRegistrationOfNothingKeepsThePartyWaiting(t *testing.T) {
	h := newScoreHarness(t, 0)
	if got := h.registerScore(partyLeader(func() (int, bool, bool) { return 0, false, false })); got.Message != noOfferingsText {
		t.Fatalf("registration without offerings = %+v, want %q", got, noOfferingsText)
	}
	if got := h.registerScore(partyLeader(func() (int, bool, bool) { return 2, true, false })); got.Page != "" || got.Message != "" || got.Aborted {
		t.Fatalf("registration of offerings not taken = %+v, want an empty reply", got)
	}
	if got := h.registerScore(partyLeader(func() (int, bool, bool) { return 3, true, true })); got.Page != "festival_3c.htm" || got.Contribution != 15 {
		t.Fatalf("registration = page %q contribution %d, want festival_3c.htm 15", got.Page, got.Contribution)
	}
	if got := h.registerScore(partyLeader(func() (int, bool, bool) { return 4, true, true })); got.Page != "festival_3b.htm" {
		t.Fatalf("registration again = page %q, want festival_3b.htm", got.Page)
	}
}

// A party whose registration registered nothing after the parties stopped
// waiting does not start waiting again.
func TestScoreRegistrationOfNothingAfterTheWaitEnds(t *testing.T) {
	h := newScoreHarness(t, 0)
	h.registerScore(partyLeader(func() (int, bool, bool) {
		h.m.mu.Lock()
		h.m.resetParticipantsLocked()
		h.m.mu.Unlock()
		return 0, false, false
	}))
	h.m.mu.Lock()
	defer h.m.mu.Unlock()
	if got := h.m.previous[side(sevensigns.Dawn)][0]; len(got) != 0 {
		t.Fatalf("party waiting = %v, want none", got)
	}
}

// Concurrent registrations of one run register it once.
func TestScoreConcurrentRegistrationsRegisterOnce(t *testing.T) {
	h := newScoreHarness(t, 0)
	var taken atomic.Int32
	leader := partyLeader(func() (int, bool, bool) {
		taken.Add(1)
		return 1, true, true
	})
	var wg sync.WaitGroup
	var registered atomic.Int32
	for range 8 {
		wg.Go(func() {
			if h.registerScore(leader).Contribution > 0 {
				registered.Add(1)
			}
		})
	}
	wg.Wait()
	if registered.Load() != 1 || taken.Load() != 1 {
		t.Fatalf("%d registrations, offerings taken %d times; want 1 and 1", registered.Load(), taken.Load())
	}
	if got := h.calendar.festivalScore[side(sevensigns.Dawn)].Load(); got != int64(MaxScores[0]) {
		t.Fatalf("Dawn festival score = %d, want %d", got, MaxScores[0])
	}
}
