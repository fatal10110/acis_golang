package sevensigns

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
)

// castlesJournal adds the castle calls to a journal of notices and
// festival calls.
type castlesJournal struct{ *journal }

func (j castlesJournal) ResetCertificates() {
	j.entries = append(j.entries, "certificates reset")
}

func (j castlesJournal) ValidateTaxes(maxPercent int) {
	j.entries = append(j.entries, fmt.Sprintf("taxes capped at %d", maxPercent))
}

// TestCastleTaxCap pins CastleManager.validateTaxes's caps by the Seal of
// Strife's owner: DAWN 25, DUSK 5, NORMAL 15.
func TestCastleTaxCap(t *testing.T) {
	for owner, want := range map[Cabal]int{Dawn: 25, Dusk: 5, NoCabal: 15} {
		if got := castleTaxCap(owner); got != want {
			t.Errorf("castleTaxCap(%v) = %d, want %d", owner, got, want)
		}
	}
}

// TestPeriodChangesActOnCastles pins the castles' part of the period
// changes (SevenSignsPeriodChange.run): recruiting's end gives every
// castle its certificates back and the competition's end caps the tax
// rates by the Seal of Strife's new owner, both ahead of the status save;
// results and seal validation leave the castles alone.
func TestPeriodChangesActOnCastles(t *testing.T) {
	for _, tt := range []struct {
		name      string
		dawnVotes [3]int
		duskVotes [3]int
		dawnScore float64
		duskScore float64
		wantCap   int
	}{
		// Dawn wins and 100% of its members chose Strife: Dawn owns it.
		{"dawn takes strife", [3]int{0, 0, 1}, [3]int{}, 10, 0, 25},
		// Dusk wins and 100% of its members chose Strife: Dusk owns it.
		{"dusk takes strife", [3]int{}, [3]int{0, 0, 1}, 0, 10, 5},
		// Dawn wins, but none of its members chose Strife: no owner.
		{"strife unowned", [3]int{1, 0, 0}, [3]int{}, 10, 0, 15},
	} {
		t.Run(tt.name, func(t *testing.T) {
			start := at(2026, time.August, 24, 17, 0, time.UTC)
			h := newStateHarness(t, start)
			j := castlesJournal{&journal{store: h.store}}
			h.state = NewState(h.store, j, h.state.log, func() time.Time { return h.current }, func(d time.Duration, fn func()) *time.Timer {
				h.fired = append(h.fired, fn)
				return nil
			})
			h.store.row = StatusRow{
				Cycle: 4, Period: Recruiting, LastSave: start,
				DawnStoneScore: tt.dawnScore, DuskStoneScore: tt.duskScore,
				DawnSealVotes: tt.dawnVotes, DuskSealVotes: tt.duskVotes,
			}
			h.store.found = true
			h.store.players = []PlayerRow{{ObjectID: 1, Cabal: Dawn}, {ObjectID: 2, Cabal: Dusk}}
			if err := h.state.Restore(context.Background()); err != nil {
				t.Fatal(err)
			}
			h.state.SetFestival(j)
			h.state.SetCastles(j)
			h.state.Start()

			status := func(n int) string { return fmt.Sprintf("festival status after %d status saves", n) }
			for _, step := range []struct {
				name string
				want string
				save int
			}{
				{"recruiting ends", "certificates reset", 1},
				{"competition ends", fmt.Sprintf("taxes capped at %d", tt.wantCap), 2},
				{"results end", "", 3},
				{"seal validation ends", "", 4},
			} {
				j.entries = nil
				h.fired[len(h.fired)-1]()
				var castleCalls []string
				for _, e := range j.entries {
					if e == "certificates reset" || strings.HasPrefix(e, "taxes capped") {
						castleCalls = append(castleCalls, e)
					}
				}
				want := []string(nil)
				if step.want != "" {
					want = []string{step.want}
				}
				if !slices.Equal(castleCalls, want) {
					t.Fatalf("%s: castle calls %q, want %q (journal %q)", step.name, castleCalls, want, j.entries)
				}
				if step.want != "" {
					i, k := slices.Index(j.entries, step.want), slices.Index(j.entries, status(step.save))
					if i < 0 || k < 0 || i > k {
						t.Fatalf("%s: castle call not ahead of the status save: %q", step.name, j.entries)
					}
				}
			}
		})
	}
}
