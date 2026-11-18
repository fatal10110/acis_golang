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
// Broadcaster, acts on them through Online (the Seal of Strife skills, the
// Seven Signs dungeons), and is saved in full.
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

	"github.com/fatal10110/acis_golang/internal/gameserver/persist"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
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

// Name is the period's display name, as a castle chamberlain's report
// shows it.
func (p Period) Name() string {
	switch p {
	case Recruiting:
		return "Quest Event Initialization"
	case Competition:
		return "Competition (Quest Event)"
	case Results:
		return "Quest Event Results"
	case SealValidation:
		return "Seal Validation"
	default:
		return p.String()
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

// The seal stones, the Seven Signs' currency, and the certificate of
// approval a castle lord hands out for joining Dawn.
const (
	BlueStoneID             int32 = 6360
	GreenStoneID            int32 = 6361
	RedStoneID              int32 = 6362
	CertificateOfApprovalID int32 = 6388
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

// Festival is the Festival of Darkness as the period changes drive it.
// Its methods are called without the state's lock held. A method taking
// then calls it once, from any goroutine, when what it started is done.
type Festival interface {
	// CompetitionBegun starts the festival schedule as recruiting ends.
	CompetitionBegun()
	// CompetitionEnded stops it as the competition ends, once the end is
	// announced and before the seals' new owners are.
	CompetitionEnded(then func())
	// CycleBegun resets the festival for cycle as seal validation ends,
	// once the end is announced.
	CycleBegun(cycle int, then func())
	// SaveStatus writes the festival's columns of the status row; every
	// status save calls it right after writing the row.
	SaveStatus(ctx context.Context) error
}

// Timer is an armed period-change timer.
type Timer interface {
	Stop() bool
}

// Writer runs the period changes' saves off the queue, in the order they
// were handed to it per owner.
type Writer interface {
	Enqueue(ownerID int32, job func()) bool
}

// State tracks the active period and the competition's scores and drives
// the period transitions. All methods are safe for concurrent use.
//
// A period change runs as a chain of steps on queue, the service queue it
// shares with the festival: each step that waits on the players' queues or
// the database hands the next step to them and returns.
type State struct {
	store     Store
	out       Broadcaster
	festival  Festival
	online    Online
	castles   Castles
	spawns    Spawns
	now       func() time.Time
	queue     *sim.Queue
	writes    Writer
	afterFunc func(time.Duration, func()) Timer
	log       zerolog.Logger

	// saveMu serializes every write to the store, so the write that lands
	// last carries the latest state: each save takes its snapshot while
	// holding it.
	saveMu sync.Mutex

	mu         sync.Mutex
	row        StatusRow
	players    map[int32]*PlayerRow
	nextChange time.Time
	timer      Timer
	// stopped latches Stop: a transition already running when Stop arrives
	// ends once its save completes, without re-arming the timer.
	stopped bool
	// transitioning is set from the start of a period change to its last
	// step; idle, on mu, is signalled when it clears.
	transitioning bool
	idle          sync.Cond
}

// NewState returns a state persisting through store and announcing period
// changes through out (nil announces nothing). Its period changes run on
// queue and save through writes. now supplies wall time; afterFunc
// schedules the next transition timer (both overridden in tests; nil falls
// back to time.Now and queue's timers). A nil queue runs every step on the
// goroutine finishing the one before, a nil writes saves there too.
func NewState(store Store, out Broadcaster, log zerolog.Logger, now func() time.Time, queue *sim.Queue, writes Writer, afterFunc func(time.Duration, func()) Timer) *State {
	if now == nil {
		now = time.Now
	}
	if afterFunc == nil {
		afterFunc = func(d time.Duration, fn func()) Timer { return queue.After(d, fn) }
	}
	s := &State{store: store, out: out, now: now, queue: queue, writes: writes, afterFunc: afterFunc, log: log, players: map[int32]*PlayerRow{}}
	s.idle.L = &s.mu
	return s
}

// SetFestival hands the period changes and status saves to festival. It
// must be called before Start.
func (s *State) SetFestival(festival Festival) {
	s.festival = festival
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

// Stop cancels the pending transition timer and latches, then waits for a
// period change already under way: it still saves the new period, but
// neither expels, shows the sky, swaps the NPC groups nor re-arms. Once Stop
// returns, the period changes neither post to a queue nor write. It must
// not run on the state's queue.
func (s *State) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopped = true
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	for s.transitioning {
		s.idle.Wait()
	}
}

// WaitIdle returns once no period change is under way. It must not run on
// the state's queue.
func (s *State) WaitIdle() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for s.transitioning {
		s.idle.Wait()
	}
}

// Save writes every sign-up and then the status row, stamping the status
// with the time of the write, then the festival's status columns.
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

	err := errors.Join(s.store.SavePlayers(ctx, players), s.store.SaveStatus(ctx, row))
	if s.festival != nil {
		err = errors.Join(err, s.festival.SaveStatus(ctx))
	}
	return err
}

// SaveStatus writes the status row alone, stamping it with the time of the
// write, then the festival's status columns.
func (s *State) SaveStatus(ctx context.Context) error {
	s.saveMu.Lock()
	defer s.saveMu.Unlock()

	s.mu.Lock()
	s.row.LastSave = s.now()
	row := s.row
	s.mu.Unlock()

	err := s.store.SaveStatus(ctx, row)
	if s.festival != nil {
		err = errors.Join(err, s.festival.SaveStatus(ctx))
	}
	return err
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

// transition is one period change as its steps carry it out.
type transition struct {
	ended, period Period
	cycle         int
	notices       []Notice
	online        Online
	castles       Castles
	spawns        Spawns
	strifeOwner   Cabal
}

// advance moves the state into the following period — settling the
// competition, or starting the next cycle after validation ends — then
// runs the change's steps in order: it announces it, driving the festival
// and the Seal of Strife skills, settles the castles, saves it, sends the
// players no longer allowed in a Seven Signs dungeon out, shows the new
// sky, swaps the Seven Signs NPC groups, and re-arms the timer. Each step
// waiting on the players or the database continues on the queue.
func (s *State) advance() {
	s.mu.Lock()
	if s.stopped || s.transitioning {
		s.mu.Unlock()
		return
	}
	s.transitioning = true
	t := &transition{ended: s.row.Period}
	t.notices = s.changePeriodLocked()
	s.nextChange = nextPeriodChange(s.row.Period, s.now())
	t.cycle, t.period = s.row.Cycle, s.row.Period
	t.online, t.castles, t.spawns = s.online, s.castles, s.spawns
	t.strifeOwner = s.strifeOwnerLocked()
	s.mu.Unlock()

	s.announce(t, s.onQueue(func() { s.save(t) }))
}

// save settles the castles and saves the period change on the writer;
// once written, the change continues on the queue. The save runs even
// when Stop arrived meanwhile, so the stored period matches the one in
// memory.
func (s *State) save(t *transition) {
	s.settleCastles(t.ended, t.castles, t.strifeOwner)
	job := func() {
		ctx, cancel := context.WithTimeout(context.Background(), saveTimeout)
		defer cancel()
		if err := s.Save(ctx); err != nil {
			s.log.Error().Err(err).Int("cycle", t.cycle).Str("period", t.period.String()).Msg("save seven signs")
		}
		s.post(func() { s.expel(t) })
	}
	if s.writes == nil || !s.writes.Enqueue(persist.ServiceOwner, job) {
		job()
	}
}

// expel sends the players no longer allowed in a Seven Signs dungeon out.
func (s *State) expel(t *transition) {
	if s.endIfStopped() {
		return
	}
	if t.online == nil {
		s.reveal(t)
		return
	}
	t.online.ExpelFromDungeons(s.onQueue(func() { s.reveal(t) }))
}

// reveal shows the new sky, swaps the Seven Signs NPC groups and re-arms
// the timer, ending the period change.
func (s *State) reveal(t *transition) {
	if s.endIfStopped() {
		return
	}
	s.broadcast([]Notice{{Kind: NoticeSky, Cabal: s.Sky()}})
	if t.spawns != nil {
		t.spawns.SevenSignsChanged()
	}
	s.log.Info().Int("cycle", t.cycle).Str("period", t.period.String()).Msg("seven signs period begun")

	s.mu.Lock()
	defer s.mu.Unlock()
	s.endLocked()
	s.scheduleLocked()
}

// endIfStopped ends the period change under way when Stop has arrived.
func (s *State) endIfStopped() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.stopped {
		return false
	}
	s.endLocked()
	return true
}

func (s *State) endLocked() {
	s.transitioning = false
	s.idle.Broadcast()
}

// onQueue returns a continuation that runs fn on the queue.
func (s *State) onQueue(fn func()) func() {
	return func() { s.post(fn) }
}

// post runs fn on the queue; on the caller when there is none or it no
// longer takes tasks, so a period change under way at shutdown still ends.
func (s *State) post(fn func()) {
	if s.queue == nil || !s.queue.Post(fn) {
		fn()
	}
}

// announce announces the notices of the period that ended, with the
// festival's change and the Seal of Strife skills at their place among
// them, then runs then. Recruiting's end starts the festival before
// anything is announced; the competition's end stops it once its sound
// and end are announced; results' end gives the Seal of Strife skills
// before anything is announced; seal validation's end takes them away once
// everything is announced, then resets the festival for the new cycle.
func (s *State) announce(t *transition, then func()) {
	notices := t.notices
	switch t.ended {
	case Recruiting:
		if s.festival != nil {
			s.festival.CompetitionBegun()
		}
		s.broadcast(notices)
		then()
	case Competition:
		split := min(2, len(notices))
		s.broadcast(notices[:split])
		rest := func() {
			s.broadcast(notices[split:])
			then()
		}
		if s.festival == nil {
			rest()
			return
		}
		s.festival.CompetitionEnded(s.onQueue(rest))
	case Results:
		announce := func() {
			s.broadcast(notices)
			then()
		}
		if t.online == nil {
			announce()
			return
		}
		t.online.GiveStrifeSkills(s.onQueue(announce))
	case SealValidation:
		s.broadcast(notices)
		reset := func() {
			if s.festival == nil {
				then()
				return
			}
			s.festival.CycleBegun(t.cycle, s.onQueue(then))
		}
		if t.online == nil {
			reset()
			return
		}
		t.online.RemoveStrifeSkills(s.onQueue(reset))
	}
}

// settleCastles applies what the ended period settles for the castles: the
// end of recruiting gives every castle its certificates back, and the end
// of the competition caps their tax rates by the Seal of Strife's new
// owner. Neither sends anything, so it follows the announcements.
func (s *State) settleCastles(ended Period, castles Castles, strifeOwner Cabal) {
	if castles == nil {
		return
	}
	switch ended {
	case Recruiting:
		castles.ResetCertificates()
	case Competition:
		castles.ValidateTaxes(CastleTaxCap(strifeOwner))
	}
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
// is Monday before 18:00. A minor period ends fifteen minutes after now; at
// a restart a saved minor period is already due (see changeAlreadyDue), so
// it ends at once instead of running fifteen minutes again.
func nextPeriodChange(period Period, now time.Time) time.Time {
	switch period {
	case Competition, SealValidation:
		days := (int(time.Monday) - int(now.Weekday()) + 7) % 7
		if days == 0 && (now.Hour() > periodStartHour || (now.Hour() == periodStartHour && now.Minute() >= periodStartMin)) {
			days = 7
		}
		return periodBoundary(now, days)
	default:
		return now.Add(periodMinorLength)
	}
}

// periodBoundary is the time of day a major period changes, 18:00 local,
// on the date days after now's.
func periodBoundary(now time.Time, days int) time.Time {
	return time.Date(now.Year(), now.Month(), now.Day(), periodStartHour, periodStartMin, 0, 0, now.Location()).AddDate(0, 0, days)
}

// changeAlreadyDue reports whether the active period ended while the server
// was down, i.e. whether its scheduled end lies before the last status save.
// A never-saved row (the schema default) never qualifies. A minor period's
// end is measured from the last save itself, so every saved minor period
// qualifies: a restart during one ends it at once.
func changeAlreadyDue(row StatusRow, now time.Time) bool {
	if row.LastSave.IsZero() || row.LastSave.UnixMilli() <= 7 {
		return false
	}
	var scheduledEnd time.Time
	switch row.Period {
	case Competition, SealValidation:
		offset := (int(now.Weekday()) - int(time.Monday) + 7) % 7
		thisMonday := periodBoundary(now, -offset)
		if now.Before(thisMonday) {
			// A week of elapsed hours back, not a calendar week: across a
			// daylight saving change the boundary lands an hour off 18:00.
			thisMonday = thisMonday.Add(-7 * 24 * time.Hour)
		}
		scheduledEnd = thisMonday
	default:
		scheduledEnd = row.LastSave.Add(periodMinorLength)
	}
	return row.LastSave.Before(scheduledEnd)
}
