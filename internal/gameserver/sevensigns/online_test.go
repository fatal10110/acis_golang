package sevensigns

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"
)

// onlineJournal adds the players-online calls to a journal of notices and
// festival calls.
type onlineJournal struct{ *journal }

func (j onlineJournal) GiveStrifeSkills()   { j.entries = append(j.entries, "strife given") }
func (j onlineJournal) RemoveStrifeSkills() { j.entries = append(j.entries, "strife removed") }
func (j onlineJournal) ExpelFromDungeons()  { j.entries = append(j.entries, "dungeons swept") }

// Each period change acts on the players online at the reference's point
// (SevenSignsPeriodChange.run): results' end gives the Seal of Strife skills
// before its sound; seal validation's end takes them away after its message
// and before the festival resets; and every change sweeps the dungeons once
// it is saved, ahead of the new sky.
func TestPeriodChangesActOnPlayersOnline(t *testing.T) {
	start := at(2026, time.August, 24, 17, 0, time.UTC)
	h := newStateHarness(t, start)
	j := onlineJournal{&journal{store: h.store}}
	h.state = NewState(h.store, j, h.state.log, func() time.Time { return h.current }, func(d time.Duration, fn func()) *time.Timer {
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
	h.state.SetOnline(j)
	h.state.Start()

	notice := func(k NoticeKind) string { return fmt.Sprintf("notice %d", k) }
	sound, sky := notice(NoticeSound), notice(NoticeSky)
	status := func(n int) string { return fmt.Sprintf("festival status after %d status saves", n) }
	for _, step := range []struct {
		name string
		want []string
	}{
		{"recruiting ends", []string{"festival begun", sound, notice(NoticeCompetitionBegun), status(1), "dungeons swept", sky}},
		{"competition ends", []string{
			sound, notice(NoticeCompetitionEnded), "festival ended",
			notice(NoticeSealObtained), notice(NoticeCabalWon), status(2), "dungeons swept", sky,
		}},
		{"results end", []string{"strife given", sound, notice(NoticeValidationBegun), status(3), "dungeons swept", sky}},
		{"seal validation ends", []string{sound, notice(NoticeValidationEnded), "strife removed", "festival cycle 5", status(4), "dungeons swept", sky}},
	} {
		j.entries = nil
		h.fired[len(h.fired)-1]()
		if !slices.Equal(j.entries, step.want) {
			t.Fatalf("%s:\n got  %q\n want %q", step.name, j.entries, step.want)
		}
	}
}

// strifeState restores a state in period with Dawn leading the score, the
// Seal of Strife owned by strifeOwner, and players 1 (Dawn), 2 (Dusk) and 3
// (signed up, no cabal); player 4 never signed up.
func strifeState(t *testing.T, period Period, strifeOwner Cabal) *State {
	t.Helper()
	h := newStateHarness(t, at(2026, time.August, 26, 12, 0, time.UTC))
	h.store.row = StatusRow{Cycle: 2, Period: period, DawnStoneScore: 10, SealOwners: [3]Cabal{NoCabal, NoCabal, strifeOwner}}
	h.store.found = true
	h.store.players = []PlayerRow{{ObjectID: 1, Cabal: Dawn}, {ObjectID: 2, Cabal: Dusk}, {ObjectID: 3}}
	if err := h.state.Restore(context.Background()); err != nil {
		t.Fatal(err)
	}
	return h.state
}

// giveSosEffect gives a cabal member the victor's skill when its cabal owns
// the Seal of Strife and the vanquished's otherwise, even with the seal
// unowned; nobody outside a cabal gets one.
func TestStrifeSkillAsValidationBegins(t *testing.T) {
	for _, tc := range []struct {
		owner Cabal
		want  [4]StrifeSkill
	}{
		{Dawn, [4]StrifeSkill{VictorOfWar, VanquishedOfWar, NoStrifeSkill, NoStrifeSkill}},
		{Dusk, [4]StrifeSkill{VanquishedOfWar, VictorOfWar, NoStrifeSkill, NoStrifeSkill}},
		{NoCabal, [4]StrifeSkill{VanquishedOfWar, VanquishedOfWar, NoStrifeSkill, NoStrifeSkill}},
	} {
		s := strifeState(t, SealValidation, tc.owner)
		for i, want := range tc.want {
			if got := s.StrifeSkill(int32(i + 1)); got != want {
				t.Errorf("strife owner %v: StrifeSkill(%d) = %v, want %v", tc.owner, i+1, got, want)
			}
		}
	}
}

// EnterWorld gives the skill only during seal validation with the seal
// owned; otherwise the player loses both.
func TestEnterStrifeSkill(t *testing.T) {
	for _, tc := range []struct {
		name   string
		period Period
		owner  Cabal
		ok     bool
		want   [4]StrifeSkill
	}{
		{"validation, dawn owns", SealValidation, Dawn, true, [4]StrifeSkill{VictorOfWar, VanquishedOfWar, NoStrifeSkill, NoStrifeSkill}},
		{"validation, unowned", SealValidation, NoCabal, false, [4]StrifeSkill{}},
		{"competition, dawn owns", Competition, Dawn, false, [4]StrifeSkill{}},
		{"results, dawn owns", Results, Dawn, false, [4]StrifeSkill{}},
	} {
		s := strifeState(t, tc.period, tc.owner)
		for i, want := range tc.want {
			got, ok := s.EnterStrifeSkill(int32(i + 1))
			if got != want || ok != tc.ok {
				t.Errorf("%s: EnterStrifeSkill(%d) = (%v, %v), want (%v, %v)", tc.name, i+1, got, ok, want, tc.ok)
			}
		}
	}
}

// The period-change sweep and the login check decide differently outside
// results and validation (teleLosingCabalFromDungeons against
// Player.onPlayerEnter): the sweep sends cabal members out and keeps a
// sign-up without a cabal, the login check the reverse. A player who never
// signed up leaves on a change in any period; at login it counts as in no
// cabal. Dawn wins every period here.
func TestDungeonExpulsionRules(t *testing.T) {
	for _, tc := range []struct {
		period Period
		change [4]bool
		login  [4]bool
	}{
		{Recruiting, [4]bool{true, true, false, true}, [4]bool{false, false, true, true}},
		{Competition, [4]bool{true, true, false, true}, [4]bool{false, false, true, true}},
		{Results, [4]bool{false, true, true, true}, [4]bool{false, true, true, true}},
		{SealValidation, [4]bool{false, true, true, true}, [4]bool{false, true, true, true}},
	} {
		s := strifeState(t, tc.period, NoCabal)
		for i := range 4 {
			id := int32(i + 1)
			if got := s.ExpelledAtPeriodChange(id); got != tc.change[i] {
				t.Errorf("%v: ExpelledAtPeriodChange(%d) = %v, want %v", tc.period, id, got, tc.change[i])
			}
			if got := s.ExpelledAtLogin(id); got != tc.login[i] {
				t.Errorf("%v: ExpelledAtLogin(%d) = %v, want %v", tc.period, id, got, tc.login[i])
			}
		}
	}
}

// With no winner (a tie), results and validation keep only the players in
// no cabal, sign-up or not at login, and only the sign-ups without one on a
// period change.
func TestDungeonExpulsionOnTie(t *testing.T) {
	h := newStateHarness(t, at(2026, time.August, 26, 12, 0, time.UTC))
	h.store.row = StatusRow{Cycle: 2, Period: SealValidation}
	h.store.found = true
	h.store.players = []PlayerRow{{ObjectID: 1, Cabal: Dawn}, {ObjectID: 3}}
	if err := h.state.Restore(context.Background()); err != nil {
		t.Fatal(err)
	}
	for id, want := range map[int32][2]bool{1: {true, true}, 3: {false, false}, 4: {true, false}} {
		if got := [2]bool{h.state.ExpelledAtPeriodChange(id), h.state.ExpelledAtLogin(id)}; got != want {
			t.Errorf("tie: expelled(%d) at change, at login = %v, want %v", id, got, want)
		}
	}
}
