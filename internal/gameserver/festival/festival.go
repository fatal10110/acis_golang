// Package festival owns the Festival of Darkness, the Seven Signs
// competition in which a party of one cabal fights a timed wave of
// monsters in one of five level-ranged arenas for the cabal's festival
// score.
//
// It keeps each festival's best score per cabal and Seven Signs cycle, the
// festival cycle counter and the accumulated bonus of each festival, and
// persists them; and it runs the festival schedule during the competition
// period, which the Seven Signs period changes start, stop and reset.
//
// The festival itself is not in place yet (#223): parties cannot sign up,
// so no festival ever runs, no score is ever set and no bonus ever
// accumulates; the schedule still counts down to each next festival.
package festival

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
	"github.com/rs/zerolog"
)

// Count is the number of festivals, one per level range.
const Count = 5

// MaxScores is the festival score each festival is worth to the cabal
// setting its best result, indexed by festival id: levels up to 31, 42,
// 53 and 64, then no limit.
var MaxScores = [Count]int{60, 70, 100, 120, 150}

// Score is one festival's best result for one cabal in one Seven Signs
// cycle, as the seven_signs_festival table keeps it.
type Score struct {
	FestivalID int
	Cabal      sevensigns.Cabal
	Cycle      int
	// Date is when the score was set, in Unix milliseconds; 0 for none.
	Date  int64
	Score int
	// Members names the party that set the score, comma-separated.
	Members string
}

// Status is the festival's part of the Seven Signs status row: the number
// of festival cycles run this competition and each festival's accumulated
// bonus.
type Status struct {
	FestivalCycle int
	Bonuses       [Count]int
}

// Store persists the festival scores and the festival's status columns.
type Store interface {
	LoadScores(ctx context.Context) ([]Score, error)
	// SaveScores inserts each score, or updates its date, score and
	// members when its festival, cabal and cycle are already stored.
	SaveScores(ctx context.Context, scores []Score) error
	// LoadStatus returns the status columns, found=false when the status
	// row does not exist.
	LoadStatus(ctx context.Context) (Status, bool, error)
	SaveStatus(ctx context.Context, status Status) error
}

// Calendar is the Seven Signs calendar the festival follows.
type Calendar interface {
	CurrentPeriod() sevensigns.Period
	NextChange() time.Time
}

// Config is the festival schedule's events.properties settings.
type Config struct {
	// ManagerStart is how long after the schedule starts its first
	// festival cycle begins.
	ManagerStart time.Duration
	// Length is how long a festival lasts.
	Length time.Duration
	// CycleLength is the length of a whole festival cycle: the signup
	// period, the festival and a minute's pause.
	CycleLength time.Duration
}

// DefaultConfig returns the shipped settings: the first cycle two minutes
// after the start, 18-minute festivals in 38-minute cycles.
func DefaultConfig() Config {
	return Config{ManagerStart: 2 * time.Minute, Length: 18 * time.Minute, CycleLength: 38 * time.Minute}
}

// signup is how long parties may sign up before each festival.
func (c Config) signup() time.Duration {
	return c.CycleLength - c.Length - time.Minute
}

// saveTimeout bounds a save the festival makes on its own.
const saveTimeout = 10 * time.Second

// Manager is the Festival of Darkness. All methods are safe for concurrent
// use; the Seven Signs period changes drive it through CompetitionBegun,
// CompetitionEnded and CycleBegun.
type Manager struct {
	cfg       Config
	store     Store
	calendar  Calendar
	now       func() time.Time
	afterFunc func(time.Duration, func()) *time.Timer
	log       zerolog.Logger

	// saveMu serializes the writes to the store, so the write that lands
	// last carries the latest state: each save takes its snapshot while
	// holding it.
	saveMu sync.Mutex

	// mu guards the fields below. The calendar may be read while holding
	// it; it is never held across a store call.
	mu         sync.Mutex
	signsCycle int
	status     Status
	// scores holds each Seven Signs cycle's scores keyed by festival id,
	// plus Count for Dawn's.
	scores map[int]map[int]Score
	schedule
}

// New returns a festival persisting through store and following calendar.
// now supplies wall time and afterFunc schedules its timers (both
// overridden in tests; nil falls back to time.Now and time.AfterFunc).
func New(cfg Config, store Store, calendar Calendar, log zerolog.Logger, now func() time.Time, afterFunc func(time.Duration, func()) *time.Timer) *Manager {
	if now == nil {
		now = time.Now
	}
	if afterFunc == nil {
		afterFunc = time.AfterFunc
	}
	return &Manager{cfg: cfg, store: store, calendar: calendar, now: now, afterFunc: afterFunc, log: log, scores: map[int]map[int]Score{}}
}

// Restore loads the stored scores and status columns; signsCycle is the
// current Seven Signs cycle, whose scores the festival reports. A missing
// status row leaves the festival cycle and the bonuses at zero.
func (m *Manager) Restore(ctx context.Context, signsCycle int) error {
	rows, err := m.store.LoadScores(ctx)
	if err != nil {
		return fmt.Errorf("load festival scores: %w", err)
	}
	status, _, err := m.store.LoadStatus(ctx)
	if err != nil {
		return fmt.Errorf("load festival status: %w", err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.signsCycle = signsCycle
	m.status = status
	m.scores = map[int]map[int]Score{}
	for _, s := range rows {
		cycle := m.scores[s.Cycle]
		if cycle == nil {
			cycle = map[int]Score{}
			m.scores[s.Cycle] = cycle
		}
		cycle[scoreKey(s.Cabal, s.FestivalID)] = s
	}
	return nil
}

// scoreKey is where a cycle's scores keep cabal's score of festivalID.
func scoreKey(cabal sevensigns.Cabal, festivalID int) int {
	if cabal == sevensigns.Dawn {
		return festivalID + Count
	}
	return festivalID
}

// HighestScore returns cabal's best result in festivalID this Seven Signs
// cycle, ok=false when none is kept for the cycle.
func (m *Manager) HighestScore(cabal sevensigns.Cabal, festivalID int) (Score, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.scores[m.signsCycle][scoreKey(cabal, festivalID)]
	return s, ok
}

// CycleBegun resets the festival for the Seven Signs cycle that began: no
// festival cycle run, no bonus accumulated, and a blank score for each
// festival and cabal, saved with every stored score.
func (m *Manager) CycleBegun(cycle int) {
	m.mu.Lock()
	m.status = Status{}
	m.signsCycle = cycle
	blank := make(map[int]Score, 2*Count)
	for id := range Count {
		for _, cabal := range [...]sevensigns.Cabal{sevensigns.Dusk, sevensigns.Dawn} {
			blank[scoreKey(cabal, id)] = Score{FestivalID: id, Cabal: cabal, Cycle: cycle}
		}
	}
	m.scores[cycle] = blank
	m.mu.Unlock()

	// ponytail: the reset also takes every unused blood offering from the
	// players online, which only the festival hands out (#223).
	ctx, cancel := context.WithTimeout(context.Background(), saveTimeout)
	defer cancel()
	if err := m.SaveScores(ctx); err != nil {
		m.log.Error().Err(err).Int("cycle", cycle).Msg("festival: save the new cycle's scores")
	}
	m.log.Info().Int("cycle", cycle).Msg("festival: reinitialized for the next competition period")
}

// SaveScores writes every kept score.
func (m *Manager) SaveScores(ctx context.Context) error {
	m.saveMu.Lock()
	defer m.saveMu.Unlock()
	m.mu.Lock()
	var rows []Score
	for _, cycle := range m.scores {
		rows = slices.AppendSeq(rows, maps.Values(cycle))
	}
	m.mu.Unlock()
	slices.SortFunc(rows, func(a, b Score) int {
		return cmp.Or(cmp.Compare(a.Cycle, b.Cycle), cmp.Compare(scoreKey(a.Cabal, a.FestivalID), scoreKey(b.Cabal, b.FestivalID)))
	})
	return m.store.SaveScores(ctx, rows)
}

// SaveStatus writes the festival cycle and the bonuses into the status
// row. Every Seven Signs status save calls it.
func (m *Manager) SaveStatus(ctx context.Context) error {
	m.saveMu.Lock()
	defer m.saveMu.Unlock()
	m.mu.Lock()
	status := m.status
	m.mu.Unlock()
	return m.store.SaveStatus(ctx, status)
}

// MemberNames splits Members into names at each comma, dropping trailing
// empty names; a score without members names one empty member.
func (s Score) MemberNames() []string {
	if s.Members == "" {
		return []string{""}
	}
	names := strings.Split(s.Members, ",")
	for len(names) > 0 && names[len(names)-1] == "" {
		names = names[:len(names)-1]
	}
	return names
}
