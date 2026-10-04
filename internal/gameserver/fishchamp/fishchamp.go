// Package fishchamp runs the weekly Fishing King Championship.
//
// Every fish a player catches is measured, 60 to 90 long (up to 3 more on a
// prize-winning lure), and the five longest catches of the week, one per
// player, stand in the running ranking. A week ends at 19:00 on a Tuesday,
// server time: its five fishers become the winners, each able to claim the
// prize of their place from a Fishing Guild Member until the next week ends,
// and a new week starts empty. The end date persists in server_memo and the
// fishers in fishing_championship; a week found over at boot ends at once.
package fishchamp

import (
	"cmp"
	"context"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/rnd"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/rs/zerolog"
)

// TaskTimeout bounds one database read or write.
const TaskTimeout = 10 * time.Second

// writeLane is the persistence owner every championship save is queued
// under. No character, item or clan has object id 0.
const writeLane int32 = 0

const (
	// Places is how many fishers stand in a ranking and win a prize.
	Places = 5
	// endHour is the hour of the Tuesday a week ends at.
	endHour = 19
	// refreshDelay is how long a running ranking a player looked at stays
	// as it was taken before the next look takes it anew.
	refreshDelay = time.Minute
	// noLength is the shortest catch of an empty ranking.
	noLength = 99999.
	// prizeLureFirst and prizeLureLast are the prize-winning lures, which
	// can make a catch up to 3 longer.
	prizeLureFirst, prizeLureLast = 8484, 8486
)

// Reward is where a fisher stands: in the running week, a winner yet to
// claim the prize, or a winner who claimed it.
type Reward int32

const (
	RewardNone      Reward = 0 // a fisher of the running week
	RewardUnclaimed Reward = 1
	RewardClaimed   Reward = 2
)

// Config is the championship's configuration.
type Config struct {
	// Enabled runs the championship; otherwise no catch is measured and no
	// ranking is kept.
	Enabled bool
	// RewardItemID is the item the prizes are paid in.
	RewardItemID int32
	// Rewards is how many of RewardItemID each place, first to fifth, wins.
	Rewards [Places]int32
}

// DefaultConfig returns the configuration used when no setting overrides
// it.
func DefaultConfig() Config {
	return Config{
		Enabled:      true,
		RewardItemID: 57,
		Rewards:      [Places]int32{800000, 500000, 300000, 200000, 100000},
	}
}

// Entry is one fisher: a row of fishing_championship.
type Entry struct {
	Name   string
	Length float64
	Reward Reward
}

// Store persists the week's end and its fishers.
type Store interface {
	// Load returns the stored end of the running week, Unix milliseconds,
	// 0 when none is stored, and every stored fisher in stored order.
	Load(ctx context.Context) (end int64, entries []Entry, err error)
	// Save stores the end of the running week and replaces the stored
	// fishers with entries.
	Save(ctx context.Context, end int64, entries []Entry) error
}

// Writer runs a database job later, on ownerID's persistence lane, so the
// championship never waits on the database.
type Writer interface {
	Enqueue(ownerID int32, job func()) bool
}

// Catch is a measured fish.
type Catch struct {
	Length float64
	// Registered is set when the catch entered the running ranking, or
	// lengthened the fisher's own catch in it.
	Registered bool
}

// Standing is one place of a ranking as the pages show it: "None" and "0"
// for a place nobody holds.
type Standing struct {
	Name, Length string
}

// Option adjusts a Championship.
type Option func(*Championship)

// WithRoll measures catches with roll, which returns an int in [0, n). The
// default rolls at random.
func WithRoll(roll func(n int) int) Option {
	return func(c *Championship) { c.roll = roll }
}

// Championship is the running week and the last week's winners. Its
// calendar runs on its queue and hands its saves to its writer; every
// method is safe from any goroutine.
type Championship struct {
	cfg    Config
	store  Store
	writes Writer
	log    zerolog.Logger
	queue  *sim.Queue
	roll   func(n int) int
	loc    *time.Location

	// saveMu serializes saves, each of which writes the state as it is
	// when the save runs, so a later save never lands an older state.
	saveMu sync.Mutex

	// mu guards the fields below it; never held across I/O.
	mu sync.Mutex
	// end is the end of the running week, Unix milliseconds.
	end int64
	// running and winners are the fishers of the running week and the
	// last week's winners, the winners longest first.
	running, winners []*Entry
	// minLength is the shortest catch of the running ranking, noLength
	// while it is empty.
	minLength float64
	// shown is the running ranking as a player last took it; stale is set
	// once it is due to be taken anew.
	shown []Standing
	stale bool
	// savePending is set while a save is queued that has not taken its
	// state yet.
	savePending bool
	// saveWanted is set by a change that unlock is to queue a save for.
	saveWanted bool
	stopped    bool
}

// New returns a championship persisting through store, a save queued on
// writes after every change; without writes it saves only when it stops,
// and without store never. Its calendar runs on queue, which it owns
// from then on, in the time zone of the queue's clock. Restore then Start
// bring it up.
func New(cfg Config, store Store, writes Writer, queue *sim.Queue, log zerolog.Logger, opts ...Option) *Championship {
	c := &Championship{
		cfg: cfg, store: store, writes: writes, log: log, queue: queue,
		roll: rnd.Get, loc: queue.Now().Location(), minLength: noLength, stale: true,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Config returns the championship's configuration.
func (c *Championship) Config() Config { return c.cfg }

// Enabled reports whether the championship runs.
func (c *Championship) Enabled() bool { return c.cfg.Enabled }

// Restore loads the stored week end and fishers: the running week's, and
// the last week's winners with whether each claimed the prize. A row of
// any other reward state is left out. It does nothing when the
// championship is disabled.
func (c *Championship) Restore(ctx context.Context) error {
	if !c.cfg.Enabled {
		return nil
	}
	end, entries, err := c.store.Load(ctx)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.end = end
	c.running, c.winners = nil, nil
	for i := range entries {
		switch e := &entries[i]; {
		case e.Reward == RewardNone:
			c.running = append(c.running, e)
		case e.Reward > RewardNone:
			c.winners = append(c.winners, e)
		}
	}
	sortLongestFirst(c.winners)
	c.recalculateMin()
	c.log.Info().Int("running", len(c.running)).Int("winners", len(c.winners)).Msg("fishing championship: restored")
	return nil
}

// Start ends the restored week at once when it is over, and otherwise
// schedules its end. It does nothing when the championship is disabled.
func (c *Championship) Start() {
	if !c.cfg.Enabled {
		return
	}
	c.mu.Lock()
	defer c.unlock()
	if now := c.now(); c.end <= now {
		c.end = now
		c.finish()
		return
	}
	c.scheduleFinish()
}

// Stop cancels the calendar, then saves the championship as it stands. A
// save already queued still lands; it writes the same state.
func (c *Championship) Stop(ctx context.Context) error {
	if !c.cfg.Enabled {
		return nil
	}
	c.queue.Close()
	c.mu.Lock()
	c.stopped = true
	c.mu.Unlock()
	return c.save(ctx)
}

// now is the queue clock in Unix milliseconds.
func (c *Championship) now() int64 { return c.queue.Now().UnixMilli() }

// scheduleFinish ends the running week at its end date. Runs under mu.
func (c *Championship) scheduleFinish() {
	delay := time.Duration(max(c.end-c.now(), 0)) * time.Millisecond
	c.queue.After(delay, func() {
		c.mu.Lock()
		defer c.unlock()
		if !c.stopped {
			c.finish()
		}
	})
}

// finish ends the running week: its fishers, longest first, become the
// winners in place of the last week's, whether or not those claimed their
// prizes, and the next week runs until the following Tuesday's end. Runs
// under mu.
func (c *Championship) finish() {
	c.winners = c.winners[:0]
	for _, e := range c.running {
		e.Reward = RewardUnclaimed
		c.winners = append(c.winners, e)
	}
	c.running = nil
	sortLongestFirst(c.winners)
	c.end = NextEnd(c.end, c.loc)
	c.queueSave()
	c.log.Info().Msg("fishing championship: a new week started")
	c.scheduleFinish()
}

// NextEnd returns the week end following from, Unix milliseconds: 19:00 on
// the Tuesday of the week, running Sunday to Saturday, that holds the day
// six days after from. The milliseconds of from are kept.
func NextEnd(from int64, loc *time.Location) int64 {
	t := time.UnixMilli(from).In(loc)
	y, m, d := t.Date()
	day := time.Date(y, m, d+6, 12, 0, 0, 0, loc)
	ms := t.Nanosecond() / int(time.Millisecond) * int(time.Millisecond)
	return time.Date(y, m, d+6+int(time.Tuesday-day.Weekday()), endHour, 0, 0, ms, loc).UnixMilli()
}

// NewFish measures the fish name caught on lureID and ranks it: a fisher
// already in the running ranking keeps the longer of its two catches, a
// new one joins while fewer than five stand, and otherwise takes the place
// of the shortest catch when it is longer. ok is false, nothing measured,
// when the championship is disabled.
func (c *Championship) NewFish(name string, lureID int32) (catch Catch, ok bool) {
	if !c.cfg.Enabled {
		return Catch{}, false
	}
	c.mu.Lock()
	defer c.unlock()
	length := float64(60+c.roll(30)) + float64(c.roll(1001))/1000.
	if lureID >= prizeLureFirst && lureID <= prizeLureLast {
		length += float64(c.roll(3001)) / 1000.
	}
	catch.Length = length
	if len(c.running) >= Places && c.minLength >= length {
		return catch, true
	}
	for _, e := range c.running {
		if strings.EqualFold(e.Name, name) {
			if e.Length < length {
				e.Length = length
				catch.Registered = true
				c.recalculateMin()
				c.queueSave()
			}
			return catch, true
		}
	}
	if len(c.running) >= Places {
		shortest := 0
		for i, e := range c.running {
			if e.Length < c.running[shortest].Length {
				shortest = i
			}
		}
		c.running = slices.Delete(c.running, shortest, shortest+1)
	}
	c.running = append(c.running, &Entry{Name: name, Length: length})
	catch.Registered = true
	c.recalculateMin()
	c.queueSave()
	return catch, true
}

// recalculateMin sets minLength to the shortest running catch. Runs under
// mu.
func (c *Championship) recalculateMin() {
	c.minLength = noLength
	for _, e := range c.running {
		c.minLength = min(c.minLength, e.Length)
	}
}

// MinutesLeft returns the whole minutes left in the running week.
func (c *Championship) MinutesLeft() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return (c.end - c.now()) / time.Minute.Milliseconds()
}

// Winners returns the last week's winners' places.
func (c *Championship) Winners() [Places]Standing {
	c.mu.Lock()
	defer c.mu.Unlock()
	ranked := make([]Standing, len(c.winners))
	for i, e := range c.winners {
		ranked[i] = Standing{Name: e.Name, Length: lengthText(e.Length)}
	}
	return standings(ranked)
}

// IsWinner reports whether name, exactly as spelled, is among the last
// week's winners, claimed or not.
func (c *Championship) IsWinner(name string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, e := range c.winners {
		if e.Name == name {
			return true
		}
	}
	return false
}

// Claim hands name its prizes: each winner entry of name, in any case, not
// yet claimed is marked claimed, and pays the reward of name's place, none
// outside the first five. It returns what each such entry pays, in winner
// order.
func (c *Championship) Claim(name string) []int32 {
	c.mu.Lock()
	defer c.unlock()
	var paid []int32
	for _, e := range c.winners {
		if e.Reward == RewardClaimed || !strings.EqualFold(e.Name, name) {
			continue
		}
		var count int32
		for place, w := range c.winners {
			if place < Places && strings.EqualFold(w.Name, name) {
				count = c.cfg.Rewards[place]
			}
		}
		e.Reward = RewardClaimed
		paid = append(paid, count)
	}
	if paid != nil {
		c.queueSave()
	}
	return paid
}

// Running returns the running ranking as a player sees it: the ranking as
// it was last taken, unless it is due to be taken anew. Then refreshing is
// set, the ranking is taken for the looks to come within a minute, and the
// places are not given.
func (c *Championship) Running() (places [Places]Standing, refreshing bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stale {
		c.stale = false
		sortLongestFirst(c.running)
		c.shown = c.shown[:0]
		for _, e := range c.running {
			c.shown = append(c.shown, Standing{Name: e.Name, Length: lengthText(e.Length)})
		}
		c.queue.After(refreshDelay, func() {
			c.mu.Lock()
			c.stale = true
			c.mu.Unlock()
		})
		return places, true
	}
	return standings(c.shown), false
}

// standings returns the first five of ranked, the places nobody holds
// filled with "None" and "0".
func standings(ranked []Standing) [Places]Standing {
	var out [Places]Standing
	for i := range out {
		out[i] = Standing{Name: "None", Length: "0"}
		if i < len(ranked) {
			out[i] = ranked[i]
		}
	}
	return out
}

// sortLongestFirst orders entries longest first, keeping the order of
// equal catches.
func sortLongestFirst(entries []*Entry) {
	slices.SortStableFunc(entries, func(a, b *Entry) int { return cmp.Compare(b.Length, a.Length) })
}

// snapshot returns the end and every fisher as they are saved: the winners
// longest first, then the running week's. Runs under mu.
func (c *Championship) snapshot() (int64, []Entry) {
	out := make([]Entry, 0, len(c.winners)+len(c.running))
	for _, e := range c.winners {
		out = append(out, *e)
	}
	for _, e := range c.running {
		out = append(out, Entry{Name: e.Name, Length: e.Length, Reward: RewardNone})
	}
	return c.end, out
}

// queueSave asks for a save once mu is released by unlock. Runs under mu.
func (c *Championship) queueSave() { c.saveWanted = true }

// unlock releases mu, then queues the save a change under it asked for.
// The save is queued only once mu is free: a writer may run its job
// inline, and the job takes mu.
func (c *Championship) unlock() {
	wanted := c.saveWanted
	c.saveWanted = false
	c.mu.Unlock()
	if wanted {
		c.enqueueSave()
	}
}

// enqueueSave queues a save of the championship unless one is queued that
// has yet to take its state. Runs without mu.
func (c *Championship) enqueueSave() {
	if c.writes == nil || c.store == nil {
		return
	}
	c.mu.Lock()
	pending := c.savePending
	c.savePending = true
	c.mu.Unlock()
	if pending {
		return
	}
	if !c.writes.Enqueue(writeLane, func() {
		ctx, cancel := context.WithTimeout(context.Background(), TaskTimeout)
		defer cancel()
		if err := c.save(ctx); err != nil {
			c.log.Error().Err(err).Msg("fishing championship: save")
		}
	}) {
		c.mu.Lock()
		c.savePending = false
		c.mu.Unlock()
		c.log.Error().Msg("fishing championship: save dropped")
	}
}

// save writes the championship as it stands now.
func (c *Championship) save(ctx context.Context) error {
	if c.store == nil {
		return nil
	}
	c.saveMu.Lock()
	defer c.saveMu.Unlock()
	c.mu.Lock()
	c.savePending = false
	end, entries := c.snapshot()
	c.mu.Unlock()
	return c.store.Save(ctx, end, entries)
}
