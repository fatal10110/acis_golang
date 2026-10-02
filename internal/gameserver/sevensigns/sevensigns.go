// Package sevensigns owns the Seven Signs competition: which of the four
// recurring periods is active in the current cycle and when it ends, the
// players signed up for each cabal with their chosen seal and contribution,
// the cabals' stone and festival scores, the owners of the three seals, and
// the persistence of all of it across restarts. The period timeline is fixed:
//
//	RECRUITING -> COMPETITION -> RESULTS -> SEAL_VALIDATION -> RECRUITING (next cycle)
//
// The competition and validation periods end at 18:00 local time on the next
// Monday; recruiting and results are short intervals lasting fifteen minutes
// from the moment they begin.
//
// Every period change is announced to the players online through a
// Broadcaster and saved in full.
package sevensigns

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

// Period is one of the four phases a Seven Signs cycle passes through. The
// numeric order is the phase order; values wrap modulo periodCount.
type Period int

const (
	Recruiting Period = iota
	Competition
	Results
	SealValidation
	periodCount
)

// String returns the persisted enum name of p.
func (p Period) String() string {
	switch p {
	case Recruiting:
		return "RECRUITING"
	case Competition:
		return "COMPETITION"
	case Results:
		return "RESULTS"
	case SealValidation:
		return "SEAL_VALIDATION"
	default:
		return fmt.Sprintf("Period(%d)", int(p))
	}
}

// ParsePeriod parses a persisted enum name into p.
func ParsePeriod(name string) (Period, error) {
	for p := Period(0); p < periodCount; p++ {
		if p.String() == name {
			return p, nil
		}
	}
	return 0, fmt.Errorf("unknown seven signs period %q", name)
}

const (
	// A major period (competition or validation) ends at 18:00 local time.
	periodStartHour = 18
	periodStartMin  = 0
	// A minor period (recruiting or results) lasts fifteen minutes.
	periodMinorLength = 15 * time.Minute
)

// StatusRow is the persisted Seven Signs status: the current cycle and
// period, when the status was last written, the last competition's winner,
// the cabals' scores, and each seal's owner and votes. The per-seal arrays
// are indexed in Seals order.
type StatusRow struct {
	Cycle    int
	Period   Period
	LastSave time.Time

	PreviousWinner    Cabal
	DawnStoneScore    float64
	DuskStoneScore    float64
	DawnFestivalScore int
	DuskFestivalScore int
	SealOwners        [3]Cabal
	// DawnSealVotes and DuskSealVotes count each cabal's sign-ups that
	// chose each seal.
	DawnSealVotes [3]int
	DuskSealVotes [3]int
}

// PlayerRow is one player's persisted sign-up: cabal and seal, the stones
// turned in this cycle by color, the ancient adena they are worth to collect,
// and the player's contribution score.
type PlayerRow struct {
	ObjectID          int32
	Cabal             Cabal
	Seal              Seal
	RedStones         int
	GreenStones       int
	BlueStones        int
	AncientAdena      int
	ContributionScore int
}

// Store persists the Seven Signs status row and the players' sign-ups.
type Store interface {
	LoadStatus(ctx context.Context) (StatusRow, bool, error)
	SaveStatus(ctx context.Context, row StatusRow) error
	LoadPlayers(ctx context.Context) ([]PlayerRow, error)
	InsertPlayer(ctx context.Context, row PlayerRow) error
	SavePlayers(ctx context.Context, rows []PlayerRow) error
}

// Broadcaster delivers period-change notices, in order, to every player
// online.
type Broadcaster interface {
	Broadcast(notices []Notice)
}

// NoticeKind names what a period-change notice tells the players.
type NoticeKind int

const (
	// NoticeSound plays Notice.Sound.
	NoticeSound NoticeKind = iota
	NoticeCompetitionBegun
	NoticeCompetitionEnded
	// NoticeSealObtained: Notice.Cabal obtained Notice.Seal.
	NoticeSealObtained
	// NoticeCabalWon: Notice.Cabal won the competition.
	NoticeCabalWon
	NoticeValidationBegun
	NoticeValidationEnded
	// NoticeSky shows the sky of Notice.Cabal, the regular sky for NoCabal.
	NoticeSky
)

// Notice is one announcement of a period change.
type Notice struct {
	Kind  NoticeKind
	Sound string
	Cabal Cabal
	Seal  Seal
}

// Period-change sounds.
const (
	soundNeutral = "SSQ_Neutral_01"
	soundDawn    = "SSQ_Dawn_01"
	soundDusk    = "SSQ_Dusk_01"
)

// saveTimeout bounds the save that follows each period change.
const saveTimeout = 10 * time.Second

// State tracks the active period and the competition's scores and drives
// the period transitions. All methods are safe for concurrent use.
type State struct {
	store     Store
	out       Broadcaster
	now       func() time.Time
	afterFunc func(time.Duration, func()) *time.Timer
	log       zerolog.Logger

	// saveMu serializes every write to the store, so the write that lands
	// last carries the latest state: each save takes its snapshot while
	// holding it.
	saveMu sync.Mutex

	mu         sync.Mutex
	row        StatusRow
	players    map[int32]*PlayerRow
	nextChange time.Time
	timer      *time.Timer
	// stopped latches Stop: a transition already running when Stop arrives
	// must not re-arm the timer once its save completes.
	stopped bool
}

// NewState returns a state persisting through store and announcing period
// changes through out (nil announces nothing). now supplies wall time;
// afterFunc schedules the next transition timer (both overridden in tests;
// nil falls back to time.Now and time.AfterFunc).
func NewState(store Store, out Broadcaster, log zerolog.Logger, now func() time.Time, afterFunc func(time.Duration, func()) *time.Timer) *State {
	if now == nil {
		now = time.Now
	}
	if afterFunc == nil {
		afterFunc = time.AfterFunc
	}
	return &State{store: store, out: out, now: now, afterFunc: afterFunc, log: log, players: map[int32]*PlayerRow{}}
}

// Restore loads the persisted status and sign-ups and computes when the
// active period ends. When the persisted save predates the moment the
// current period should have ended, the transition is marked due
// immediately: Start fires it without waiting. The database default row
// (cycle 1, competition, never saved) is assumed when none was written yet.
func (s *State) Restore(ctx context.Context) error {
	players, err := s.store.LoadPlayers(ctx)
	if err != nil {
		return fmt.Errorf("load seven signs players: %w", err)
	}
	row, found, err := s.store.LoadStatus(ctx)
	if err != nil {
		return fmt.Errorf("load seven signs status: %w", err)
	}
	if !found {
		row = StatusRow{Cycle: 1, Period: Competition}
	}

	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.row = row
	s.players = make(map[int32]*PlayerRow, len(players))
	for i := range players {
		p := players[i]
		s.players[p.ObjectID] = &p
	}
	if changeAlreadyDue(row, now) {
		s.nextChange = now
	} else {
		s.nextChange = nextPeriodChange(row.Period, now)
	}
	s.log.Info().Str("period", row.Period.String()).Int("cycle", row.Cycle).Int("players", len(players)).
		Str("leading", s.winningCabalLocked().String()).Msg("seven signs restored")
	return nil
}

// Start arms the transition timer for the pending period change and clears
// any earlier Stop. Restore must have completed first.
func (s *State) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopped = false
	s.scheduleLocked()
}

// Stop cancels the pending transition timer and latches, so a transition
// already in flight cannot re-arm once it finishes persisting.
func (s *State) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopped = true
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
}

// Save writes every sign-up and then the status row, stamping the status
// with the time of the write.
func (s *State) Save(ctx context.Context) error {
	s.saveMu.Lock()
	defer s.saveMu.Unlock()

	s.mu.Lock()
	s.row.LastSave = s.now()
	row := s.row
	players := make([]PlayerRow, 0, len(s.players))
	for _, p := range s.players {
		players = append(players, *p)
	}
	s.mu.Unlock()
	slices.SortFunc(players, func(a, b PlayerRow) int { return cmp.Compare(a.ObjectID, b.ObjectID) })

	return errors.Join(s.store.SavePlayers(ctx, players), s.store.SaveStatus(ctx, row))
}

// CurrentPeriod returns the active period.
func (s *State) CurrentPeriod() Period {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.row.Period
}

// CurrentCycle returns the current cycle number.
func (s *State) CurrentCycle() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.row.Cycle
}

// NextChange reports when the active period ends.
func (s *State) NextChange() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.nextChange
}

// advance moves the state into the following period — settling the
// competition, or starting the next cycle after validation ends — announces
// it, persists it, shows the new sky, and re-arms the timer.
func (s *State) advance() {
	s.mu.Lock()
	notices := s.changePeriodLocked()
	s.nextChange = nextPeriodChange(s.row.Period, s.now())
	cycle, period := s.row.Cycle, s.row.Period
	s.mu.Unlock()

	s.broadcast(notices)

	ctx, cancel := context.WithTimeout(context.Background(), saveTimeout)
	defer cancel()
	if err := s.Save(ctx); err != nil {
		s.log.Error().Err(err).Int("cycle", cycle).Str("period", period.String()).Msg("save seven signs")
	}

	s.broadcast([]Notice{{Kind: NoticeSky, Cabal: s.Sky()}})
	s.log.Info().Int("cycle", cycle).Str("period", period.String()).Msg("seven signs period begun")

	s.mu.Lock()
	defer s.mu.Unlock()
	s.scheduleLocked()
}

func (s *State) broadcast(notices []Notice) {
	if s.out != nil && len(notices) > 0 {
		s.out.Broadcast(notices)
	}
}

// changePeriodLocked enters the period following the active one, applies
// what the ended period settles, and returns the notices announcing it.
func (s *State) changePeriodLocked() []Notice {
	ended := s.row.Period
	s.row.Period = (ended + 1) % periodCount
	switch ended {
	case Recruiting:
		return []Notice{{Kind: NoticeSound, Sound: soundNeutral}, {Kind: NoticeCompetitionBegun}}
	case Competition:
		notices := []Notice{{Kind: NoticeSound, Sound: soundNeutral}, {Kind: NoticeCompetitionEnded}}
		winner := s.winningCabalLocked()
		notices = append(notices, s.settleSealsLocked(winner)...)
		if winner != NoCabal {
			notices = append(notices, Notice{Kind: NoticeCabalWon, Cabal: winner})
		}
		s.row.PreviousWinner = winner
		return notices
	case Results:
		sound := soundDusk
		if s.row.PreviousWinner == Dawn {
			sound = soundDawn
		}
		return []Notice{{Kind: NoticeSound, Sound: sound}, {Kind: NoticeValidationBegun}}
	default: // SealValidation: a new cycle begins.
		for _, p := range s.players {
			p.Cabal, p.Seal, p.ContributionScore = NoCabal, NoSeal, 0
		}
		s.row.DawnSealVotes, s.row.DuskSealVotes = [3]int{}, [3]int{}
		s.row.Cycle++
		s.row.DawnStoneScore, s.row.DuskStoneScore = 0, 0
		s.row.DawnFestivalScore, s.row.DuskFestivalScore = 0, 0
		return []Notice{{Kind: NoticeSound, Sound: soundNeutral}, {Kind: NoticeValidationEnded}}
	}
}

// settleSealsLocked hands each seal to its new owner after a competition
// won by winner and returns the notices of the seals a cabal obtained.
func (s *State) settleSealsLocked(winner Cabal) []Notice {
	var notices []Notice
	dawnMembers, duskMembers := s.memberCountsLocked()
	for i, seal := range Seals {
		dawn, dusk := s.sealPercentsLocked(i, dawnMembers, duskMembers)
		owner, _ := sealOutcome(s.row.SealOwners[i], winner, dawn, dusk)
		s.row.SealOwners[i] = owner
		if owner != NoCabal {
			notices = append(notices, Notice{Kind: NoticeSealObtained, Cabal: owner, Seal: seal})
		}
	}
	return notices
}

// sealPercentsLocked returns the percent of each cabal's members who chose
// the seal at index i, given each cabal's member count and counting an
// empty cabal as one member.
func (s *State) sealPercentsLocked(i, dawnMembers, duskMembers int) (dawn, dusk int) {
	return share(s.row.DawnSealVotes[i], max(1, dawnMembers), 100),
		share(s.row.DuskSealVotes[i], max(1, duskMembers), 100)
}

func (s *State) scheduleLocked() {
	if s.stopped {
		return
	}
	if s.timer != nil {
		s.timer.Stop()
	}
	delay := s.nextChange.Sub(s.now())
	if delay < 0 {
		delay = 0
	}
	s.timer = s.afterFunc(delay, s.advance)
}

// nextPeriodChange computes when period ends, given it is active at now. A
// major period ends at 18:00 local on the next Monday — today counts when it
// is Monday before 18:00. A minor period ends fifteen minutes after it
// began, measured from now: a restart during such a short interval grants it
// its full length again rather than resuming the previous countdown.
func nextPeriodChange(period Period, now time.Time) time.Time {
	switch period {
	case Competition, SealValidation:
		days := (int(time.Monday) - int(now.Weekday()) + 7) % 7
		if days == 0 && (now.Hour() > periodStartHour || (now.Hour() == periodStartHour && now.Minute() >= periodStartMin)) {
			days = 7
		}
		target := time.Date(now.Year(), now.Month(), now.Day(), periodStartHour, periodStartMin, 0, 0, now.Location())
		return target.AddDate(0, 0, days)
	default:
		return now.Add(periodMinorLength)
	}
}

// changeAlreadyDue reports whether the active period ended while the server
// was down, i.e. whether its scheduled end lies before the last status save.
// A never-saved row (the schema default) never qualifies.
func changeAlreadyDue(row StatusRow, now time.Time) bool {
	if row.LastSave.IsZero() || row.LastSave.UnixMilli() <= 7 {
		return false
	}
	var scheduledEnd time.Time
	switch row.Period {
	case Competition, SealValidation:
		offset := (int(now.Weekday()) - int(time.Monday) + 7) % 7
		thisMonday := time.Date(now.Year(), now.Month(), now.Day(), periodStartHour, periodStartMin, 0, 0, now.Location()).AddDate(0, 0, -offset)
		if now.Before(thisMonday) {
			thisMonday = thisMonday.AddDate(0, 0, -7)
		}
		scheduledEnd = thisMonday
	default:
		scheduledEnd = row.LastSave.Add(periodMinorLength)
	}
	return row.LastSave.Before(scheduledEnd)
}
