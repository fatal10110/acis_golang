package sevensigns

import (
	"context"
	"slices"
	"testing"
	"time"
)

// Expected values model the single-precision score arithmetic: a ratio is
// narrowed to a float, scaled in float, and rounded half up. 29/200 is the
// float-sensitive vector: in float 0.145f*100 is exactly 14.5 and rounds to
// 15, where double arithmetic gives 14.4999... and 14.
func TestScoreFormulas(t *testing.T) {
	for _, tc := range []struct {
		name string
		got  int
		want int
	}{
		{"dawn third of the pot", cabalScore(1, 3, 0), 167},
		{"dusk two thirds of the pot", cabalScore(2, 3, 0), 333},
		{"empty pot counts festival only", cabalScore(0, 0, 7), 7},
		{"festival adds to the stone share", cabalScore(1, 3, 40), 207},
		{"stone proportion third", stoneProportion(1, 3), 167},
		{"stone proportion two thirds", stoneProportion(2, 3), 333},
		{"stone proportion empty pot", stoneProportion(5, 0), 0},
		{"share rounds half up", share(1, 8, 100), 13},
		{"share half up again", share(3, 8, 100), 38},
		{"share half a percent", share(1, 200, 100), 1},
		{"share in float", share(29, 200, 100), 15},
		{"share of total score", share(167, 500, 100), 33},
		{"share of total score rival", share(333, 500, 100), 67},
		{"stone score", StoneScore(2, 3, 4), 2*3 + 3*5 + 4*10},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %d, want %d", tc.name, tc.got, tc.want)
		}
	}
}

func TestSealOutcome(t *testing.T) {
	for _, tc := range []struct {
		owner, winner Cabal
		dawn, dusk    int
		want          Cabal
		reason        Prediction
	}{
		{NoCabal, NoCabal, 100, 100, NoCabal, PredictionTie},
		{NoCabal, Dawn, 35, 0, Dawn, PredictionClaimed},
		{NoCabal, Dawn, 34, 100, NoCabal, PredictionNotClaimed},
		{NoCabal, Dusk, 100, 35, Dusk, PredictionClaimed},
		{NoCabal, Dusk, 100, 34, NoCabal, PredictionNotClaimed},
		{Dawn, NoCabal, 10, 0, Dawn, PredictionOwnedRetained},
		{Dawn, NoCabal, 9, 100, NoCabal, PredictionTie},
		{Dawn, Dawn, 10, 100, Dawn, PredictionOwnedRetained},
		{Dawn, Dawn, 9, 100, NoCabal, PredictionOwnedLost},
		{Dawn, Dusk, 100, 35, Dusk, PredictionClaimed},
		{Dawn, Dusk, 10, 34, Dawn, PredictionOwnedRetained},
		{Dawn, Dusk, 9, 34, NoCabal, PredictionOwnedLost},
		{Dusk, NoCabal, 0, 10, Dusk, PredictionOwnedRetained},
		{Dusk, NoCabal, 100, 9, NoCabal, PredictionTie},
		{Dusk, Dawn, 35, 100, Dawn, PredictionClaimed},
		{Dusk, Dawn, 34, 10, Dusk, PredictionOwnedRetained},
		{Dusk, Dawn, 34, 9, NoCabal, PredictionOwnedLost},
		{Dusk, Dusk, 100, 10, Dusk, PredictionOwnedRetained},
		{Dusk, Dusk, 100, 9, NoCabal, PredictionOwnedLost},
	} {
		got, reason := sealOutcome(tc.owner, tc.winner, tc.dawn, tc.dusk)
		if got != tc.want || reason != tc.reason {
			t.Errorf("sealOutcome(owner %v, winner %v, dawn %d%%, dusk %d%%) = (%v, %d), want (%v, %d)",
				tc.owner, tc.winner, tc.dawn, tc.dusk, got, reason, tc.want, tc.reason)
		}
	}
}

// restoredHarness restores h over the given status and sign-ups and arms
// its timer.
func restoredHarness(t *testing.T, row StatusRow, players ...PlayerRow) *stateHarness {
	t.Helper()
	start := at(2026, time.August, 26, 12, 0, time.UTC)
	h := newStateHarness(t, start)
	h.store.row, h.store.found, h.store.players = row, true, players
	if err := h.state.Restore(context.Background()); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	h.state.Start()
	return h
}

func TestSignUpVotesAndInsertsOnce(t *testing.T) {
	h := restoredHarness(t, StatusRow{Cycle: 1, Period: Recruiting})
	ctx := context.Background()
	if err := h.state.SetPlayerInfo(ctx, 10, Dawn, Gnosis); err != nil {
		t.Fatal(err)
	}
	// Changing an existing sign-up writes nothing at once and counts again.
	if err := h.state.SetPlayerInfo(ctx, 10, Dawn, Strife); err != nil {
		t.Fatal(err)
	}
	if err := h.state.SetPlayerInfo(ctx, 11, Dusk, Avarice); err != nil {
		t.Fatal(err)
	}
	// Anything but Dawn counts toward Dusk's votes.
	if err := h.state.SetPlayerInfo(ctx, 12, NoCabal, Avarice); err != nil {
		t.Fatal(err)
	}
	want := []PlayerRow{{ObjectID: 10, Cabal: Dawn, Seal: Gnosis}, {ObjectID: 11, Cabal: Dusk, Seal: Avarice}, {ObjectID: 12, Cabal: NoCabal, Seal: Avarice}}
	if !slices.Equal(h.store.inserted, want) {
		t.Fatalf("inserted = %+v, want %+v", h.store.inserted, want)
	}
	h.state.mu.Lock()
	dawn, dusk := h.state.row.DawnSealVotes, h.state.row.DuskSealVotes
	h.state.mu.Unlock()
	if dawn != [3]int{0, 1, 1} || dusk != [3]int{2, 0, 0} {
		t.Fatalf("votes = dawn %v dusk %v", dawn, dusk)
	}
	if h.state.PlayerCabal(10) != Dawn || h.state.PlayerSeal(10) != Strife || h.state.PlayerCabal(99) != NoCabal || h.state.PlayerSeal(99) != NoSeal {
		t.Fatal("sign-up lookups disagree")
	}
}

func TestStoneContributionCapAndReward(t *testing.T) {
	h := restoredHarness(t, StatusRow{Cycle: 1, Period: Competition},
		PlayerRow{ObjectID: 10, Cabal: Dawn, Seal: Gnosis, ContributionScore: 90},
		PlayerRow{ObjectID: 11, Cabal: Dusk, Seal: Avarice},
	)
	if _, ok := h.state.AddPlayerStoneContrib(99, 1, 0, 0, 1000); ok {
		t.Fatal("a player who never signed up turned stones in")
	}
	if pts, ok := h.state.AddPlayerStoneContrib(10, 1, 1, 1, 107); ok || pts != 0 {
		t.Fatalf("contribution over the cap = (%d, %v), want refused", pts, ok)
	}
	if pts, ok := h.state.AddPlayerStoneContrib(10, 1, 1, 0, 98); !ok || pts != 8 {
		t.Fatalf("contribution up to the cap = (%d, %v), want (8, true)", pts, ok)
	}
	if pts, ok := h.state.AddPlayerStoneContrib(11, 0, 0, 2, 1000); !ok || pts != 20 {
		t.Fatalf("dusk contribution = (%d, %v)", pts, ok)
	}
	r := h.state.Record(10)
	if r.PlayerStones != 2 || r.PlayerAncientAdena != 8 {
		t.Fatalf("record = %d stones, %d adena; want 2, 8", r.PlayerStones, r.PlayerAncientAdena)
	}
	// 8 of 28 stone points: 142.857f -> 143 for Dawn, 357.14f -> 357 for Dusk.
	if r.Dawn.StoneProportion != 143 || r.Dusk.StoneProportion != 357 || r.Winner != Dusk {
		t.Fatalf("standings = dawn %+v dusk %+v winner %v", r.Dawn, r.Dusk, r.Winner)
	}
	if got := h.state.TakeAncientAdenaReward(10); got != 8 {
		t.Fatalf("reward = %d, want 8", got)
	}
	r = h.state.Record(10)
	if r.PlayerStones != 0 || r.PlayerAncientAdena != 0 || h.state.TakeAncientAdenaReward(10) != 0 {
		t.Fatalf("reward not cleared: %+v", r)
	}
	// Collecting keeps the cabal's stone score.
	if r.Dawn.StoneProportion != 143 {
		t.Fatalf("dawn stone share after collecting = %d", r.Dawn.StoneProportion)
	}
}

func TestFestivalScoreMovesBetweenCabals(t *testing.T) {
	h := restoredHarness(t, StatusRow{Cycle: 1, Period: Competition, DawnFestivalScore: 50, DuskFestivalScore: 10})
	h.state.AddFestivalScore(Dusk, 30) // Dawn keeps 20.
	h.state.AddFestivalScore(Dawn, 50) // Dusk has only 40: nothing taken.
	r := h.state.Record(0)
	if r.Dawn.FestivalScore != 70 || r.Dusk.FestivalScore != 40 {
		t.Fatalf("festival scores = dawn %d dusk %d, want 70 40", r.Dawn.FestivalScore, r.Dusk.FestivalScore)
	}
	if r.Dawn.TotalScore != 70 || r.Dusk.TotalScore != 40 || r.Dawn.Percent != 64 || r.Dusk.Percent != 36 {
		t.Fatalf("totals = dawn %+v dusk %+v", r.Dawn, r.Dusk)
	}
}

// A competition ends: the seals go to their new owners and each obtained
// seal and the winner are announced after the period-ended notices; the
// results period then announces the winner's sound and validation; the sky
// turns to the winner's during validation; the cycle then wraps.
func TestCompetitionSettlesSealsAndCycleWraps(t *testing.T) {
	h := restoredHarness(t, StatusRow{
		Cycle: 3, Period: Competition, LastSave: at(2026, time.August, 26, 11, 0, time.UTC),
		DawnStoneScore: 30, DuskStoneScore: 10, DawnFestivalScore: 5,
		// Avarice: unowned, 2 of 4 dawn members (50%) claim it.
		// Gnosis: dusk-owned, 1 of 2 dusk members keep it; dawn 1 of 4 (25%).
		// Strife: dawn-owned, nobody voted: lost.
		SealOwners:    [3]Cabal{NoCabal, Dusk, Dawn},
		DawnSealVotes: [3]int{2, 1, 0},
		DuskSealVotes: [3]int{0, 1, 0},
	},
		PlayerRow{ObjectID: 1, Cabal: Dawn, Seal: Avarice, RedStones: 3, ContributionScore: 30, AncientAdena: 30},
		PlayerRow{ObjectID: 2, Cabal: Dawn, Seal: Avarice},
		PlayerRow{ObjectID: 3, Cabal: Dawn, Seal: Gnosis},
		PlayerRow{ObjectID: 4, Cabal: Dawn, Seal: NoSeal},
		PlayerRow{ObjectID: 5, Cabal: Dusk, Seal: Gnosis, BlueStones: 1},
		PlayerRow{ObjectID: 6, Cabal: Dusk, Seal: NoSeal},
	)
	before := h.state.Record(0)
	if before.Seals[0].Predicted != Dawn || before.Seals[1].Predicted != Dusk || before.Seals[2].Predicted != NoCabal {
		t.Fatalf("predicted owners = %+v", before.Seals)
	}

	h.current = at(2026, time.August, 31, 18, 0, time.UTC)
	h.fired[0]() // competition -> results
	want := []Notice{
		{Kind: NoticeSound, Sound: "SSQ_Neutral_01"},
		{Kind: NoticeCompetitionEnded},
		{Kind: NoticeSealObtained, Cabal: Dawn, Seal: Avarice},
		{Kind: NoticeSealObtained, Cabal: Dusk, Seal: Gnosis},
		{Kind: NoticeCabalWon, Cabal: Dawn},
		{Kind: NoticeSky, Cabal: NoCabal},
	}
	if !slices.Equal(h.out.notices, want) {
		t.Fatalf("competition end notices = %+v, want %+v", h.out.notices, want)
	}
	status := h.store.saves[len(h.store.saves)-1]
	if status.Period != Results || status.PreviousWinner != Dawn || status.SealOwners != [3]Cabal{Dawn, Dusk, NoCabal} {
		t.Fatalf("saved status = %+v", status)
	}
	if got := h.store.playerSaves[len(h.store.playerSaves)-1]; len(got) != 6 || got[0].ObjectID != 1 || got[5].ObjectID != 6 {
		t.Fatalf("saved sign-ups = %+v, want all six by object id", got)
	}

	h.out.notices = nil
	h.fired[1]() // results -> seal validation
	want = []Notice{
		{Kind: NoticeSound, Sound: "SSQ_Dawn_01"},
		{Kind: NoticeValidationBegun},
		{Kind: NoticeSky, Cabal: Dawn},
	}
	if !slices.Equal(h.out.notices, want) {
		t.Fatalf("validation start notices = %+v, want %+v", h.out.notices, want)
	}
	if h.state.Sky() != Dawn {
		t.Fatalf("validation sky = %v", h.state.Sky())
	}

	h.out.notices = nil
	h.fired[2]() // seal validation -> recruiting of cycle 4
	want = []Notice{
		{Kind: NoticeSound, Sound: "SSQ_Neutral_01"},
		{Kind: NoticeValidationEnded},
		{Kind: NoticeSky, Cabal: NoCabal},
	}
	if !slices.Equal(h.out.notices, want) {
		t.Fatalf("validation end notices = %+v, want %+v", h.out.notices, want)
	}
	status = h.store.saves[len(h.store.saves)-1]
	if status.Cycle != 4 || status.Period != Recruiting || status.DawnStoneScore != 0 || status.DuskStoneScore != 0 ||
		status.DawnFestivalScore != 0 || status.DawnSealVotes != [3]int{} || status.DuskSealVotes != [3]int{} ||
		status.SealOwners != [3]Cabal{Dawn, Dusk, NoCabal} || status.PreviousWinner != Dawn {
		t.Fatalf("status after the cycle wrap = %+v", status)
	}
	// Sign-ups lose cabal, seal and contribution; stones and adena stay.
	got := h.store.playerSaves[len(h.store.playerSaves)-1][0]
	if want := (PlayerRow{ObjectID: 1, RedStones: 3, AncientAdena: 30}); got != want {
		t.Fatalf("sign-up after the cycle wrap = %+v, want %+v", got, want)
	}
}

// A tied competition awards nothing new and announces no winner; the
// results period then plays the dusk sound and validation shows the regular
// sky.
func TestTiedCompetitionAnnouncesNoWinner(t *testing.T) {
	h := restoredHarness(t, StatusRow{Cycle: 1, Period: Competition, LastSave: at(2026, time.August, 26, 11, 0, time.UTC)})
	h.fired[0]()
	h.fired[1]()
	want := []Notice{
		{Kind: NoticeSound, Sound: "SSQ_Neutral_01"},
		{Kind: NoticeCompetitionEnded},
		{Kind: NoticeSky, Cabal: NoCabal},
		{Kind: NoticeSound, Sound: "SSQ_Dusk_01"},
		{Kind: NoticeValidationBegun},
		{Kind: NoticeSky, Cabal: NoCabal},
	}
	if !slices.Equal(h.out.notices, want) {
		t.Fatalf("notices = %+v, want %+v", h.out.notices, want)
	}
}

func TestRecruitingEndAnnouncesCompetition(t *testing.T) {
	h := restoredHarness(t, StatusRow{Cycle: 1, Period: Recruiting, LastSave: at(2026, time.August, 26, 11, 0, time.UTC)})
	h.fired[0]()
	want := []Notice{{Kind: NoticeSound, Sound: "SSQ_Neutral_01"}, {Kind: NoticeCompetitionBegun}, {Kind: NoticeSky, Cabal: NoCabal}}
	if !slices.Equal(h.out.notices, want) {
		t.Fatalf("notices = %+v, want %+v", h.out.notices, want)
	}
}

// The record's seal page counts each cabal's members separately and shows
// zero for a cabal with none; the prediction divides by one instead.
func TestRecordSealShares(t *testing.T) {
	h := restoredHarness(t, StatusRow{
		Cycle: 1, Period: Competition, DawnStoneScore: 10,
		DawnSealVotes: [3]int{1, 0, 0},
	}, PlayerRow{ObjectID: 1, Cabal: Dawn, Seal: Avarice}, PlayerRow{ObjectID: 2, Cabal: Dawn, Seal: NoSeal},
		PlayerRow{ObjectID: 3, Cabal: Dawn, Seal: NoSeal})
	r := h.state.Record(1)
	if r.PlayerCabal != Dawn || r.PlayerSeal != Avarice || r.Winner != Dawn {
		t.Fatalf("record header = %+v", r)
	}
	avarice := r.Seals[0]
	if avarice.DawnPercent != 33 || avarice.DuskPercent != 0 || avarice.Predicted != NoCabal || avarice.Prediction != PredictionNotClaimed {
		t.Fatalf("avarice = %+v", avarice)
	}
	if r.Dawn.TotalScore != 500 || r.Dawn.Percent != 100 || r.Dusk.Percent != 0 {
		t.Fatalf("standings = dawn %+v dusk %+v", r.Dawn, r.Dusk)
	}
}
