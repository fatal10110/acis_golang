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
// Parties sign up at the festival guides, paying seal stones into the
// festival's bonus. Each festival cycle, once the signup period is over,
// the parties signed up are taken into their arenas, where waves of
// monsters leave blood offerings to their leaders; at the end they are
// taken back, and each leader may register the party's offerings as its
// score. The guides and witches also answer the pages that read the scores
// and bonuses, and pay out the bonuses during seal validation.
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

// World is how the festival reaches the players, clans and arenas. Its
// methods are called without the festival's lock held.
type World interface {
	// RewardClanOf adds points to the reputation of the clan of the
	// character named name, online or not, and tells the clan's members
	// online that name earned them for the clan in the festival. A
	// character in no clan earns nothing. It returns once the members
	// have been sent all of it, so what the caller sends next follows.
	RewardClanOf(name string, points int)
	// RemoveBloodOfferings destroys every blood offering the players
	// online carry, without a word, and returns once it is done.
	RemoveBloodOfferings()
	// Shout shouts text, spoken by sender, to every player inside the
	// oracles' peace zones.
	Shout(sender, text string)
	// Locate returns where the player objectID stands; false when it is
	// not in the world.
	Locate(objectID int32) (Spot, bool)
	// Admit takes the player objectID into an arena at (x, y, z): it stops
	// what it does and lands within 20 of the point, its effects end but
	// for those lasting through death, and the blood offerings it carries
	// are destroyed, telling it. A player not in the world is skipped.
	Admit(objectID int32, x, y, z int)
	// Relocate takes the player objectID out of its arena: it stops what
	// it does and lands within 20 of at, or in the nearest town when at
	// is nil, and is told it was removed from the arena. A player not in
	// the world is skipped.
	Relocate(objectID int32, at *Spot)
	// Tell sends the player objectID text as a plain message.
	Tell(objectID int32, text string)
	// Settle returns once everything Admit, Relocate and Tell did so far
	// to each player of objectIDs in the world has been carried out. It
	// must not be called on a player's queue.
	Settle(objectIDs []int32)
	// SpawnWitch places the witch npcID at s and shows its arrival, two
	// casts on itself; false when it could not be placed.
	SpawnWitch(npcID int, s Spot) (NPC, bool)
	// Say has n say text to the players around it.
	Say(n NPC, text string)
	// SpawnMonster places the monster npcID at (x, y, z) facing a random
	// way, its kills worth bonus blood offerings; false when it could not
	// be placed. It never respawns.
	SpawnMonster(npcID, x, y, z, bonus int) (NPC, bool)
	// Swarm has n, when it is alive and idle, run to (x, y, z).
	Swarm(n NPC, x, y, z int)
	// Unspawn deletes n, alive or dead, unless it has left the world.
	Unspawn(n NPC)
}

// Spot is a point and a heading.
type Spot struct{ X, Y, Z, Heading int }

// NPC is an NPC the festival placed in an arena.
type NPC interface{ ObjectID() int32 }

// BloodOfferingID is the blood offering, the item the festival's monsters
// leave and its scores are counted in.
const BloodOfferingID = 5901

// highestRankedReward is the clan reputation each member of a festival's
// best-ever party earns for the clan as the competition ends.
const highestRankedReward = 100

// Calendar is the Seven Signs calendar the festival follows and scores
// into.
type Calendar interface {
	CurrentPeriod() sevensigns.Period
	NextChange() time.Time
	// AddFestivalScore raises cabal's festival score by amount, at the
	// rival's expense.
	AddFestivalScore(cabal sevensigns.Cabal, amount int)
	// SaveStatus writes the Seven Signs status, the festival's columns
	// with it.
	SaveStatus(ctx context.Context) error
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
	// MinPlayers is the fewest members a party signs up with and keeps
	// its run with, 2 to 9.
	MinPlayers int
	// FirstSpawn is when, counted from a festival's start, its first
	// monsters appear; FirstSwarm when they rush the arena's center;
	// SecondSpawn when the archers join; SecondSwarm when everything
	// rushes the center again; ChestSpawn when the chests appear.
	FirstSpawn, FirstSwarm, SecondSpawn, SecondSwarm, ChestSpawn time.Duration
}

// DefaultConfig returns the shipped settings: the first cycle two minutes
// after the start, 18-minute festivals in 38-minute cycles, parties of at
// least five, monsters at 2 minutes rushing at 5, archers at 9, the second
// rush at 12 and chests at 15.
func DefaultConfig() Config {
	return Config{
		ManagerStart: 2 * time.Minute, Length: 18 * time.Minute, CycleLength: 38 * time.Minute, MinPlayers: 5,
		FirstSpawn: 2 * time.Minute, FirstSwarm: 5 * time.Minute, SecondSpawn: 9 * time.Minute, SecondSwarm: 12 * time.Minute, ChestSpawn: 15 * time.Minute,
	}
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
	mu sync.Mutex
	// world is what the period changes reach; nil until SetWorld.
	world      World
	signsCycle int
	status     Status
	// scores holds each Seven Signs cycle's scores keyed by festival id,
	// plus Count for Dawn's.
	scores map[int]map[int]Score
	schedule
	run
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
	m := &Manager{cfg: cfg, store: store, calendar: calendar, now: now, afterFunc: afterFunc, log: log, scores: map[int]map[int]Score{}}
	m.run = newRun()
	return m
}

// SetWorld hands the festival the players and clans its period changes
// reach; until then they reach no one.
func (m *Manager) SetWorld(w World) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.world = w
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
	return m.highestScoreLocked(cabal, festivalID)
}

func (m *Manager) highestScoreLocked(cabal sevensigns.Cabal, festivalID int) (Score, bool) {
	s, ok := m.scores[m.signsCycle][scoreKey(cabal, festivalID)]
	return s, ok
}

// overallHighestLocked returns festivalID's best result of any cabal in any
// cycle, ok=false when no score of it is above 0. Of equal results the
// first found wins, searching the cycles in ascending order and each
// cycle's Dusk score before its Dawn one.
func (m *Manager) overallHighestLocked(festivalID int) (Score, bool) {
	var best Score
	found := false
	for _, cycle := range slices.Sorted(maps.Keys(m.scores)) {
		for _, cabal := range [...]sevensigns.Cabal{sevensigns.Dusk, sevensigns.Dawn} {
			s, ok := m.scores[cycle][scoreKey(cabal, festivalID)]
			if ok && s.Score > best.Score {
				best, found = s, true
			}
		}
	}
	return best, found
}

// CycleBegun resets the festival for the Seven Signs cycle that began: no
// festival cycle run, no bonus accumulated, no party signed up or waiting
// to register its score, and a blank score for each festival and cabal,
// saved with every stored score. Then every blood offering left on the
// players online is destroyed.
func (m *Manager) CycleBegun(cycle int) {
	m.mu.Lock()
	m.status = Status{}
	m.signsCycle = cycle
	m.resetParticipantsLocked()
	blank := make(map[int]Score, 2*Count)
	for id := range Count {
		for _, cabal := range [...]sevensigns.Cabal{sevensigns.Dusk, sevensigns.Dawn} {
			blank[scoreKey(cabal, id)] = Score{FestivalID: id, Cabal: cabal, Cycle: cycle}
		}
	}
	m.scores[cycle] = blank
	world := m.world
	m.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), saveTimeout)
	defer cancel()
	if err := m.SaveScores(ctx); err != nil {
		m.log.Error().Err(err).Int("cycle", cycle).Msg("festival: save the new cycle's scores")
	}
	if world != nil {
		world.RemoveBloodOfferings()
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
