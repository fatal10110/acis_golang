package sevensigns

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"
)

// journal records the period-change notices and the festival calls in the
// order they happen.
type journal struct {
	entries []string
	store   *recordingStore
}

func (j *journal) Broadcast(notices []Notice) {
	for _, n := range notices {
		j.entries = append(j.entries, fmt.Sprintf("notice %d", n.Kind))
	}
}

func (j *journal) CompetitionBegun() { j.entries = append(j.entries, "festival begun") }
func (j *journal) CompetitionEnded() { j.entries = append(j.entries, "festival ended") }
func (j *journal) CycleBegun(cycle int) {
	j.entries = append(j.entries, fmt.Sprintf("festival cycle %d", cycle))
}

// SaveStatus records how many status rows were written before it.
func (j *journal) SaveStatus(context.Context) error {
	j.entries = append(j.entries, fmt.Sprintf("festival status after %d status saves", len(j.store.saves)))
	return nil
}

// Each period change drives the festival at the reference's point in the
// announcements: recruiting's end starts it before anything is announced;
// the competition's end stops it after the sound and the end message, ahead
// of the seals obtained and the winner; seal validation's end resets it for
// the new cycle after its sound and message. Results' end leaves it alone.
// Every save writes the festival's status columns right after the status
// row.
func TestPeriodChangesDriveTheFestival(t *testing.T) {
	start := at(2026, time.August, 24, 17, 0, time.UTC)
	h := newStateHarness(t, start)
	j := &journal{store: h.store}
	h.state = NewState(h.store, j, h.state.log, func() time.Time { return h.current }, func(d time.Duration, fn func()) *time.Timer {
		h.delays = append(h.delays, d)
		h.fired = append(h.fired, fn)
		return nil
	})
	h.store.row = StatusRow{Cycle: 4, Period: Recruiting, LastSave: start, DawnStoneScore: 10, DawnSealVotes: [3]int{1, 0, 0}}
	h.store.found = true
	h.store.players = []PlayerRow{{ObjectID: 1, Cabal: Dawn, Seal: Avarice}}
	if err := h.state.Restore(context.Background()); err != nil {
		t.Fatal(err)
	}
	h.state.SetFestival(j)
	h.state.Start()

	sky := fmt.Sprintf("notice %d", NoticeSky)
	sound := fmt.Sprintf("notice %d", NoticeSound)
	for _, step := range []struct {
		name string
		want []string
	}{
		{"recruiting ends", []string{"festival begun", sound, fmt.Sprintf("notice %d", NoticeCompetitionBegun), "festival status after 1 status saves", sky}},
		{"competition ends", []string{
			sound, fmt.Sprintf("notice %d", NoticeCompetitionEnded), "festival ended",
			fmt.Sprintf("notice %d", NoticeSealObtained), fmt.Sprintf("notice %d", NoticeCabalWon), "festival status after 2 status saves", sky,
		}},
		{"results end", []string{sound, fmt.Sprintf("notice %d", NoticeValidationBegun), "festival status after 3 status saves", sky}},
		{"seal validation ends", []string{sound, fmt.Sprintf("notice %d", NoticeValidationEnded), "festival cycle 5", "festival status after 4 status saves", sky}},
	} {
		j.entries = nil
		h.fired[len(h.fired)-1]()
		if !slices.Equal(j.entries, step.want) {
			t.Fatalf("%s:\n got  %q\n want %q", step.name, j.entries, step.want)
		}
	}

	j.entries = nil
	if err := h.state.Save(context.Background()); err != nil {
		t.Fatal(err)
	}
	if want := []string{"festival status after 5 status saves"}; !slices.Equal(j.entries, want) {
		t.Fatalf("save: got %q, want %q", j.entries, want)
	}
}
