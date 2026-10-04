package derby

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/scheduler"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
)

// Lanes is how many monsters run one race.
const Lanes = event.DerbyLanes

// FirstRunnerID and LastRunnerID bound the npc ids of the monsters that can
// run a race.
const (
	FirstRunnerID = 31003
	LastRunnerID  = 31026
)

// TickInterval is how often the race countdown advances: one step a
// second.
const TickInterval = time.Second

// cycleEnd is the last countdown step of one race; the step after it
// starts the next race.
const cycleEnd = 1200

// storeTimeout bounds one persistence call of the track.
const storeTimeout = 10 * time.Second

// Race track system messages.
const (
	MsgTicketsAvailable      = 816 // S1 is the race number
	MsgTicketsNowAvailable   = 817 // S1 is the race number
	MsgTicketsStopIn         = 818 // S1 is minutes
	MsgTicketSalesClosed     = 819 // sent without parameter
	MsgRaceBeginsInMinutes   = 820 // S1 is minutes
	MsgRaceBeginsIn30Seconds = 821 // sent without parameter
	MsgCountdownInFive       = 822 // sent without parameter
	MsgRaceBeginsInSeconds   = 823 // S1 is seconds
	MsgRaceStart             = 824
	MsgRaceEnd               = 825 // S1 is the race number
	MsgFirstPlaceSecond      = 826 // S1 and S2 are lanes
	MsgNoPayoutInfo          = 1044
	MsgTicketsNotAvailable   = 1046
)

// Race phase codes of the race packet.
var (
	codeInitial = [2]int32{-1, 0}
	codeStart   = [2]int32{0, 15322}
	codeRunning = [2]int32{13765, -1}
)

// Race sounds.
var (
	soundRace      = event.DerbySound{Type: 1, File: "S_Race"}
	soundRaceStart = event.DerbySound{File: "ItemSound2.race_start"}
)

// raceState is where the current race stands.
type raceState int

const (
	// acceptingBets sells tickets for the current race.
	acceptingBets raceState = iota
	// waiting has closed the sales and published the odds.
	waiting
	// startingRace runs the race.
	startingRace
	// raceEnd has settled the race.
	raceEnd
)

// Runner is a monster that can run a race: its template under its own
// object id.
type Runner struct {
	ObjectID int32
	Template
}

// Template is what the track reads of a runner's npc template.
type Template struct {
	NpcID           int
	Name            string
	CollisionHeight float64
	CollisionRadius float64
}

// History is one race's record: the lane indexes (0 to 7) of its first and
// second monsters and the odds paid on its winner. The current race holds
// zeros until it ends.
type History struct {
	RaceID  int
	First   int
	Second  int
	OddRate float64
}

// Bet is a lane's (1 to 8) total stake on the current race.
type Bet struct {
	Lane   int
	Amount int64
}

// Store persists the race records and the lanes' stakes.
type Store interface {
	LoadHistory(ctx context.Context) ([]History, error)
	LoadBets(ctx context.Context) ([]Bet, error)
	SaveBet(ctx context.Context, bet Bet) error
	ClearBets(ctx context.Context) error
	SaveHistory(ctx context.Context, h History) error
}

// IDAllocator hands out world object ids.
type IDAllocator interface {
	NextID() (int32, error)
}

// Rand draws the track's random numbers.
type Rand interface {
	IntN(n int) int
	Shuffle(n int, swap func(i, j int))
}

// Track is the monster race track: the race countdown, the runners, the
// stakes on each lane and every race's record. Tick, and so every race
// change, runs on one goroutine at a time: the track's ticker in
// production, the test in a suite. mu guards the race state; saveMu orders
// the persistence of a change after the change itself, so the stored
// stakes and records follow the order the changes were made in, without
// holding mu across the store.
type Track struct {
	store Store
	sink  event.Sink
	rnd   Rand
	log   zerolog.Logger

	saveMu sync.Mutex

	mu         sync.Mutex
	runners    []Runner
	chosen     []Runner
	history    map[int]*History
	bets       [Lanes]int64
	odds       []float64
	raceNumber int
	countdown  int
	state      raceState
	race       *event.DerbyRace
	speeds     [Lanes][event.DerbySegments]int
	first      int
	second     int
}

// New builds the track: it restores every race record, which sets the
// current race number past the last one stored, and each lane's stake,
// then gives each runner template an object id. Stakes stored for a lane
// outside 1 to 8 are left out. sink, when non-nil, hears what the track
// announces.
func New(ctx context.Context, store Store, templates []Template, ids IDAllocator, sink event.Sink, rnd Rand, log zerolog.Logger) (*Track, error) {
	if store == nil || ids == nil || rnd == nil {
		return nil, errors.New("derby: nil store, id allocator or random source")
	}
	if len(templates) < Lanes {
		return nil, fmt.Errorf("derby: %d runner templates, want at least %d", len(templates), Lanes)
	}
	t := &Track{store: store, sink: sink, rnd: rnd, log: log, history: make(map[int]*History), raceNumber: 1, state: raceEnd}
	records, err := store.LoadHistory(ctx)
	if err != nil {
		return nil, err
	}
	for _, h := range records {
		t.history[h.RaceID] = &h
		if t.raceNumber <= h.RaceID {
			t.raceNumber = h.RaceID + 1
		}
	}
	bets, err := store.LoadBets(ctx)
	if err != nil {
		return nil, err
	}
	for _, b := range bets {
		if b.Lane < 1 || b.Lane > Lanes {
			log.Warn().Int("lane", b.Lane).Msg("derby: stake on an unknown lane left out")
			continue
		}
		t.bets[b.Lane-1] += b.Amount
	}
	for _, tpl := range templates {
		id, err := ids.NextID()
		if err != nil {
			return nil, fmt.Errorf("derby: runner %d: %w", tpl.NpcID, err)
		}
		t.runners = append(t.runners, Runner{ObjectID: id, Template: tpl})
	}
	log.Info().Int("records", len(t.history)).Int("race", t.raceNumber).Msg("derby track loaded")
	return t, nil
}

// Start launches the track's countdown, whose first step runs at once.
// Stopping the ticker cuts short the stores of a step in flight.
func (t *Track) Start(log zerolog.Logger) *scheduler.Ticker {
	t.Tick()
	return scheduler.StartContext(TickInterval, t.TickContext, log)
}

// RaceNumber is the current race's number.
func (t *Track) RaceNumber() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.raceNumber
}

// Runners returns the current race's runners in lane order, none before the
// first race.
func (t *Track) Runners() []Runner {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]Runner(nil), t.chosen...)
}

// RacePacket is the race packet a player coming to know a race manager is
// shown, false before the first race.
func (t *Track) RacePacket() (event.DerbyRace, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.race == nil {
		return event.DerbyRace{}, false
	}
	return *t.race, true
}

// HistoryOf returns race raceID's record.
func (t *Track) HistoryOf(raceID int) (History, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	h, ok := t.history[raceID]
	if !ok {
		return History{}, false
	}
	return *h, true
}

// Stakes returns each lane's stake on the current race, in lane order.
func (t *Track) Stakes() [Lanes]int64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.bets
}

// PlaceBet adds amount to lane's stake and stores the lane's new total. A
// lane outside 1 to 8 is refused.
func (t *Track) PlaceBet(lane int, amount int64) {
	if lane < 1 || lane > Lanes {
		return
	}
	t.saveMu.Lock()
	defer t.saveMu.Unlock()
	t.mu.Lock()
	t.bets[lane-1] += amount
	bet := Bet{Lane: lane, Amount: t.bets[lane-1]}
	t.mu.Unlock()
	t.persist(context.Background(), "save derby bet", func(ctx context.Context) error { return t.store.SaveBet(ctx, bet) })
}

// Tick runs one second of the race countdown: a new race with its
// runners, the ticket sales and their end, the countdown to the start, the
// race and its result, and the runners leaving the track, each announced to
// every player inside a track zone.
func (t *Track) Tick() { t.TickContext(context.Background()) }

// TickContext is Tick whose stores end early once ctx is done.
func (t *Track) TickContext(ctx context.Context) {
	t.saveMu.Lock()
	t.mu.Lock()
	if t.countdown > cycleEnd {
		t.countdown = 0
	}
	parts, saves := t.stepLocked()
	t.countdown++
	t.mu.Unlock()
	for _, save := range saves {
		save(ctx)
	}
	t.saveMu.Unlock()
	if len(parts) > 0 && t.sink != nil {
		t.sink.Emit(event.DerbyAnnounced{Parts: parts})
	}
}

// stepLocked runs the current countdown step, returning what it announces
// and the stores it makes, in order.
func (t *Track) stepLocked() ([]event.DerbyPart, []func(context.Context)) {
	race := func() event.DerbyPart { return *t.race }
	switch c := t.countdown; {
	case c == 0:
		t.newRaceLocked()
		t.newSpeedsLocked()
		t.state = acceptingBets
		t.race = t.racePacketLocked(codeInitial)
		return []event.DerbyPart{race(), message(MsgTicketsAvailable, t.raceNumber)}, nil
	case c == 300, c == 600, c == 840:
		minutes := map[int]int{300: 10, 600: 5, 840: 1}[c]
		return []event.DerbyPart{message(MsgTicketsNowAvailable, t.raceNumber), message(MsgTicketsStopIn, minutes)}, nil
	case c >= 30 && c <= 870 && c%30 == 0:
		return []event.DerbyPart{message(MsgTicketsNowAvailable, t.raceNumber)}, nil
	case c == 900:
		t.state = waiting
		t.calculateOddsLocked()
		return []event.DerbyPart{message(MsgTicketsNowAvailable, t.raceNumber), message(MsgTicketSalesClosed)}, nil
	case c == 960, c == 1020:
		minutes := 1
		if c == 960 {
			minutes = 2
		}
		return []event.DerbyPart{message(MsgRaceBeginsInMinutes, minutes)}, nil
	case c == 1050:
		return []event.DerbyPart{message(MsgRaceBeginsIn30Seconds)}, nil
	case c == 1070:
		return []event.DerbyPart{message(MsgCountdownInFive)}, nil
	case c >= 1075 && c <= 1079:
		return []event.DerbyPart{message(MsgRaceBeginsInSeconds, 1080-c)}, nil
	case c == 1080:
		t.state = startingRace
		t.race = t.racePacketLocked(codeStart)
		return []event.DerbyPart{message(MsgRaceStart), soundRace, soundRaceStart, race()}, nil
	case c == 1085:
		t.race = t.racePacketLocked(codeRunning)
		return []event.DerbyPart{race()}, nil
	case c == 1115:
		return t.endRaceLocked()
	case c == 1140:
		var gone event.DerbyRunnersGone
		for i, r := range t.chosen {
			gone.ObjectIDs[i] = r.ObjectID
		}
		return []event.DerbyPart{gone}, nil
	}
	return nil, nil
}

// endRaceLocked settles the current race: its record takes the winners and
// the winner's odds and is stored, every stake is cleared, and the result is
// announced. The race number moves on.
func (t *Track) endRaceLocked() ([]event.DerbyPart, []func(context.Context)) {
	t.state = raceEnd
	var saves []func(context.Context)
	if h, ok := t.history[t.raceNumber]; ok {
		h.First, h.Second = t.first, t.second
		if t.first < len(t.odds) {
			h.OddRate = t.odds[t.first]
		}
		record := *h
		saves = append(saves, func(ctx context.Context) {
			t.persist(ctx, "save derby history", func(ctx context.Context) error { return t.store.SaveHistory(ctx, record) })
		})
	}
	t.bets = [Lanes]int64{}
	saves = append(saves, func(ctx context.Context) {
		t.persist(ctx, "clear derby bets", t.store.ClearBets)
	})
	parts := []event.DerbyPart{message(MsgFirstPlaceSecond, t.first+1, t.second+1), message(MsgRaceEnd, t.raceNumber)}
	t.raceNumber++
	return parts, saves
}

// newRaceLocked opens the current race's record and draws its eight
// runners.
func (t *Track) newRaceLocked() {
	t.history[t.raceNumber] = &History{RaceID: t.raceNumber}
	t.rnd.Shuffle(len(t.runners), func(i, j int) { t.runners[i], t.runners[j] = t.runners[j], t.runners[i] })
	t.chosen = append(t.chosen[:0:0], t.runners[:Lanes]...)
}

// newSpeedsLocked draws every lane's speed over each segment, the last one
// always 100, and finds the first and second lanes: the furthest total
// wins, a later lane winning a tie. The previous winners are only replaced,
// never cleared, as the lanes are walked.
func (t *Track) newSpeedsLocked() {
	t.speeds = [Lanes][event.DerbySegments]int{}
	winnerDistance, secondDistance := 0, 0
	for i := range Lanes {
		total := 0
		for j := range event.DerbySegments {
			if j == event.DerbySegments-1 {
				t.speeds[i][j] = 100
			} else {
				t.speeds[i][j] = t.rnd.IntN(60) + 65
			}
			total += t.speeds[i][j]
		}
		switch {
		case total >= winnerDistance:
			t.second, secondDistance = t.first, winnerDistance
			t.first, winnerDistance = i, total
		case total >= secondDistance:
			t.second, secondDistance = i, total
		}
	}
}

// calculateOddsLocked publishes each lane's odds: none on a lane nobody
// bet on, else 70% of every stake shared over the lane's own, at least
// 1.25.
func (t *Track) calculateOddsLocked() {
	var sum int64
	for _, amount := range t.bets {
		sum += amount
	}
	t.odds = t.odds[:0]
	for _, amount := range t.bets {
		if amount == 0 {
			t.odds = append(t.odds, 0)
			continue
		}
		t.odds = append(t.odds, max(1.25, float64(sum)*0.7/float64(amount)))
	}
}

// racePacketLocked builds the race packet of phase code for the current
// runners and speeds.
func (t *Track) racePacketLocked(code [2]int32) *event.DerbyRace {
	race := &event.DerbyRace{Code1: code[0], Code2: code[1]}
	for i, r := range t.chosen {
		race.Runners[i] = event.DerbyRunner{ObjectID: r.ObjectID, NpcID: r.NpcID, CollisionHeight: r.CollisionHeight, CollisionRadius: r.CollisionRadius}
		for j, speed := range t.speeds[i] {
			race.Speeds[i][j] = uint8(speed)
		}
	}
	return race
}

// lastHistoryLocked returns the latest eight race records, newest first.
func (t *Track) lastHistoryLocked() []History {
	ids := make([]int, 0, len(t.history))
	for id := range t.history {
		ids = append(ids, id)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(ids)))
	if len(ids) > 8 {
		ids = ids[:8]
	}
	out := make([]History, len(ids))
	for i, id := range ids {
		out[i] = *t.history[id]
	}
	return out
}

// persist runs one store call, logging its failure: the race goes on.
func (t *Track) persist(ctx context.Context, what string, save func(ctx context.Context) error) {
	ctx, cancel := context.WithTimeout(ctx, storeTimeout)
	defer cancel()
	if err := save(ctx); err != nil {
		t.log.Error().Err(err).Msg(what)
	}
}

func message(id int, numbers ...int) event.DerbyMessage {
	m := event.DerbyMessage{ID: id}
	for _, n := range numbers {
		m.Numbers = append(m.Numbers, int32(n))
	}
	return m
}
