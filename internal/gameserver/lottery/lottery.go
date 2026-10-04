// Package lottery runs the weekly Lucky Lottery.
//
// A round sells tickets until ten minutes before its drawing, at 19:00 on a
// Saturday local to the server clock. The drawing picks five distinct
// numbers from 1 to 20, counts the round's tickets by how many they match,
// and splits the jackpot: the first three places share configured parts of
// it, less what the fourth place (one or two matching numbers) pays at a
// fixed price per ticket. The next round's jackpot is the configured base
// prize plus this one's, less one ticket's share of each of the first
// three places and the fourth place's whole payout; it goes on sale a
// minute later. Ticket sales add their price to the jackpot. Rounds and their
// drawings persist in the games table; a round found unfinished at boot
// resumes, and one at most two minutes from its drawing is drawn at once.
package lottery

import (
	"context"
	"sync"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/commons/rnd"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/rs/zerolog"
)

// TaskTimeout bounds one database read or write.
const TaskTimeout = 10 * time.Second

// writeLane is the persistence owner every lottery write is queued under:
// one lane keeps them in the order they were made. No character, item or
// clan has object id 0.
const writeLane int32 = 0

const (
	minuteMillis = int64(time.Minute / time.Millisecond)
	weekMillis   = int64(7 * 24 * time.Hour / time.Millisecond)
	// drawHour is the hour of the Saturday a round is drawn at.
	drawHour = 19
	// salesCloseMillis is how long before its drawing a round stops selling
	// tickets; a round resumed with less than salesResumeMillis left sells
	// none.
	salesCloseMillis  = 10 * minuteMillis
	salesResumeMillis = 12 * minuteMillis
	// drawNowMillis is how close to its drawing a resumed round is drawn at
	// once.
	drawNowMillis = 2 * minuteMillis
	// restartDelay is how long after a drawing the next round starts.
	restartDelay = time.Minute
)

// Config is the lottery's configuration.
type Config struct {
	// Enabled runs the lottery; otherwise no round ever starts.
	Enabled bool
	// Prize is the base jackpot each round adds to what the last one left.
	Prize int32
	// TicketPrice is what a ticket costs; it goes to the jackpot.
	TicketPrice int32
	// FiveNumberRate, FourNumberRate and ThreeNumberRate are the parts of
	// the jackpot, less the fourth place's payout, the first three places
	// share.
	FiveNumberRate, FourNumberRate, ThreeNumberRate float64
	// TwoAndOneNumberPrize is what a ticket matching one or two numbers
	// wins.
	TwoAndOneNumberPrize int32
}

// DefaultConfig returns the configuration used when no setting overrides
// it.
func DefaultConfig() Config {
	return Config{
		Enabled:              true,
		Prize:                50000,
		TicketPrice:          2000,
		FiveNumberRate:       0.6,
		FourNumberRate:       0.2,
		ThreeNumberRate:      0.2,
		TwoAndOneNumberPrize: 200,
	}
}

// StoredRound is one row of the games table.
type StoredRound struct {
	ID       int32
	Prize    int32
	NewPrize int32
	// EndDate is the round's drawing, in Unix milliseconds.
	EndDate  int64
	Finished bool
	Draw     Draw
}

// Store persists the rounds and reads the tickets sold for one.
type Store interface {
	// LoadRounds returns every stored round.
	LoadRounds(ctx context.Context) ([]StoredRound, error)
	InsertRound(ctx context.Context, id int32, endDate int64, prize int32) error
	// SavePrize sets round id's jackpot, and what passes to the next round.
	SavePrize(ctx context.Context, id, prize int32) error
	// FinishRound marks round id drawn with its jackpot, what passes to the
	// next round and its drawing.
	FinishRound(ctx context.Context, id, prize, newPrize int32, draw Draw) error
	// Tickets returns the numbers of every stored ticket of round id.
	Tickets(ctx context.Context, id int32) ([]Numbers, error)
}

// Writer runs a database job later, on ownerID's persistence lane, so the
// lottery never waits on the database.
type Writer interface {
	Enqueue(ownerID int32, job func()) bool
}

// Announcer tells every player online about the lottery.
type Announcer interface {
	// OnSale: tickets for round are on sale.
	OnSale(round int32)
	// SalesClosed: ticket sales stopped ahead of the drawing.
	SalesClosed()
	// Drawn: round was drawn with jackpot prize; winners tickets took the
	// first place.
	Drawn(round, prize, winners int32)
}

// Status is the lottery as a player sees it.
type Status struct {
	// Round is the round on sale, or the next one between rounds.
	Round int32
	// Prize is the round's jackpot.
	Prize int32
	// EndDate is the round's drawing, in Unix milliseconds.
	EndDate int64
	// Started is set while a round runs; Selling while it sells tickets.
	Started, Selling bool
}

// Option adjusts a Lottery.
type Option func(*Lottery)

// WithRoll draws the winning numbers with roll, which returns an int in
// [0, n). The default rolls at random.
func WithRoll(roll func(n int) int) Option {
	return func(l *Lottery) { l.roll = roll }
}

// Lottery is the running round and the drawings of past ones. Its calendar
// steps run on its queue and hand their database work to its writer; the
// reads are safe from any goroutine.
type Lottery struct {
	cfg    Config
	store  Store
	writes Writer
	out    Announcer
	log    zerolog.Logger
	queue  *sim.Queue
	roll   func(n int) int
	loc    *time.Location

	// run serializes the calendar steps with Stop. It guards the fields
	// below it.
	run     sync.Mutex
	stopped bool
	// last is the newest round, as the games table holds it.
	last      StoredRound
	lastFound bool

	// saves orders the jackpot saves of concurrent ticket sales: each takes
	// its value and queues its save under it, so the saves land in the
	// order the jackpot grew.
	saves sync.Mutex

	// mu guards the fields below it; never held across I/O or an
	// announcement.
	mu     sync.Mutex
	status Status
	draws  map[int32]Draw
}

// New returns a lottery persisting through store, its database work queued
// on writes (run inline when nil), and announcing through out. Its calendar
// runs on queue, which it owns from then on, in the time zone of the queue's
// clock. Restore then Start bring it up.
func New(cfg Config, store Store, writes Writer, out Announcer, queue *sim.Queue, log zerolog.Logger, opts ...Option) *Lottery {
	now := queue.Now()
	l := &Lottery{
		cfg: cfg, store: store, writes: writes, out: out, log: log, queue: queue,
		roll: rnd.Get, loc: now.Location(), draws: map[int32]Draw{},
		status: Status{Round: 1, Prize: cfg.Prize, EndDate: now.UnixMilli()},
	}
	for _, opt := range opts {
		opt(l)
	}
	return l
}

// Config returns the lottery's configuration.
func (l *Lottery) Config() Config { return l.cfg }

// Restore loads the stored rounds: the newest one, which Start resumes or
// follows, and every drawing a ticket can be checked against.
func (l *Lottery) Restore(ctx context.Context) error {
	rounds, err := l.store.LoadRounds(ctx)
	if err != nil {
		return err
	}
	l.run.Lock()
	defer l.run.Unlock()
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, r := range rounds {
		l.draws[r.ID] = r.Draw
		if !l.lastFound || r.ID > l.last.ID {
			l.last, l.lastFound = r, true
		}
	}
	l.log.Info().Int("rounds", len(rounds)).Int32("last", l.last.ID).Msg("lottery: restored")
	return nil
}

// Start resumes or starts a round, when the lottery is enabled.
func (l *Lottery) Start() {
	if !l.cfg.Enabled {
		return
	}
	l.queue.Post(func() {
		l.run.Lock()
		defer l.run.Unlock()
		if !l.stopped {
			l.startRound()
		}
	})
}

// Stop cancels the pending calendar steps and waits for a running one. The
// writes already queued land when the writer drains; a drawing whose
// tickets are still being read is dropped, and is drawn again at the next
// boot.
func (l *Lottery) Stop() {
	l.queue.Close()
	l.run.Lock()
	defer l.run.Unlock()
	l.stopped = true
}

// Status returns the lottery as it stands.
func (l *Lottery) Status() Status {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.status
}

// FormatDate writes ms, Unix milliseconds, as the lottery pages show a
// drawing's date: "Oct 10, 2026", in the lottery's time zone.
func (l *Lottery) FormatDate(ms int64) string {
	return time.UnixMilli(ms).In(l.loc).Format("Jan 2, 2006")
}

// IncreasePrize adds count, a sold ticket's price, to the running round's
// jackpot and queues its save.
func (l *Lottery) IncreasePrize(count int32) {
	l.saves.Lock()
	defer l.saves.Unlock()
	l.mu.Lock()
	l.status.Prize += count
	round, prize := l.status.Round, l.status.Prize
	l.mu.Unlock()
	l.write("save jackpot", func(ctx context.Context, st Store) error { return st.SavePrize(ctx, round, prize) })
}

// Check returns what a ticket of round with numbers won, and the adena its
// place pays: the drawing's prize for the first three places, the fixed
// prize for the fourth. A round never drawn wins nothing.
func (l *Lottery) Check(round int32, numbers Numbers) (Place, int32) {
	l.mu.Lock()
	draw, ok := l.draws[round]
	l.mu.Unlock()
	if !ok {
		return 0, 0
	}
	switch n := matches(numbers, draw.Numbers); n {
	case 0:
		return 0, 0
	case 5:
		return 1, draw.Prize1
	case 4:
		return 2, draw.Prize2
	case 3:
		return 3, draw.Prize3
	default:
		return 4, l.cfg.TwoAndOneNumberPrize
	}
}

// now is the queue clock in Unix milliseconds.
func (l *Lottery) now() int64 { return l.queue.Now().UnixMilli() }

// startRound resumes the newest stored round when it is still running, draws
// it when its drawing is at most two minutes away, and otherwise puts the
// next round on sale, drawn on the coming Saturday at 19:00. Runs under run.
func (l *Lottery) startRound() {
	now := l.now()
	if l.lastFound && !l.last.Finished {
		l.mu.Lock()
		l.status.Round, l.status.Prize, l.status.EndDate = l.last.ID, l.last.Prize, l.last.EndDate
		end := l.last.EndDate
		resume := end > now+drawNowMillis
		if resume {
			l.status.Started = true
			l.status.Selling = end > now+salesResumeMillis
		}
		l.mu.Unlock()
		if !resume {
			l.finish()
			return
		}
		l.after(end-now, l.finish)
		if end > now+salesResumeMillis {
			l.after(end-now-salesCloseMillis, l.closeSales)
		}
		return
	}
	l.mu.Lock()
	if l.lastFound {
		l.status.Round, l.status.Prize = l.last.ID+1, l.last.NewPrize
	}
	l.status.Selling, l.status.Started = true, true
	round := l.status.Round
	l.mu.Unlock()

	l.out.OnSale(round)

	l.mu.Lock()
	end := nextDrawing(l.status.EndDate, l.loc)
	l.status.EndDate = end
	prize := l.status.Prize
	l.mu.Unlock()
	l.after(end-now-salesCloseMillis, l.closeSales)
	l.after(end-now, l.finish)
	l.last, l.lastFound = StoredRound{ID: round, Prize: prize, NewPrize: prize, EndDate: end}, true
	l.write("store new round", func(ctx context.Context, st Store) error { return st.InsertRound(ctx, round, end, prize) })
}

// nextDrawing returns the drawing following from, Unix milliseconds: 19:00
// on from's Saturday, from's week running Sunday to Saturday, or a week
// later when from is itself a Saturday. The milliseconds of from are kept.
func nextDrawing(from int64, loc *time.Location) int64 {
	t := time.UnixMilli(from).In(loc)
	y, m, d := t.Date()
	ms := t.Nanosecond() / int(time.Millisecond) * int(time.Millisecond)
	if t.Weekday() == time.Saturday {
		return time.Date(y, m, d, drawHour, 0, 0, ms, loc).UnixMilli() + weekMillis
	}
	return time.Date(y, m, d+int(time.Saturday-t.Weekday()), drawHour, 0, 0, ms, loc).UnixMilli()
}

// closeSales stops ticket sales ahead of the drawing. Runs under run.
func (l *Lottery) closeSales() {
	l.mu.Lock()
	l.status.Selling = false
	l.mu.Unlock()
	l.out.SalesClosed()
}

// finish draws the running round: five distinct numbers from 1 to 20, then,
// once the round's tickets are read, the prizes. Runs under run.
func (l *Lottery) finish() {
	var nums [5]int
	for i := range nums {
		for {
			n := l.roll(20) + 1
			repeated := false
			for _, prev := range nums[:i] {
				if prev == n {
					repeated = true
				}
			}
			if !repeated {
				nums[i] = n
				break
			}
		}
	}
	drawn := numbersOf(nums[:])
	round := l.Status().Round
	read := func(ctx context.Context, st Store) []Numbers {
		tickets, err := st.Tickets(ctx, round)
		if err != nil {
			l.log.Error().Err(err).Int32("round", round).Msg("lottery: read tickets")
		}
		return tickets
	}
	if l.writes == nil {
		ctx, cancel := context.WithTimeout(context.Background(), TaskTimeout)
		defer cancel()
		l.settle(round, drawn, read(ctx, l.store))
		return
	}
	// The tickets are read on the lottery's persistence lane, behind the
	// round's queued writes; the drawing then goes on on the queue. A
	// ticket's own row is at most one item-persistence tick behind its
	// sale, and sales close ten minutes before the drawing.
	store := l.store
	job := func() {
		ctx, cancel := context.WithTimeout(context.Background(), TaskTimeout)
		defer cancel()
		tickets := read(ctx, store)
		l.queue.Post(func() {
			l.run.Lock()
			defer l.run.Unlock()
			if !l.stopped {
				l.settle(round, drawn, tickets)
			}
		})
	}
	if !l.writes.Enqueue(writeLane, job) {
		l.log.Error().Int32("round", round).Msg("lottery: read tickets: job dropped")
	}
}

// settle pays out round, drawn as drawn with tickets sold: it counts the
// winners of each place, announces the drawing, stores it, and starts the
// next round a minute later. Runs under run.
func (l *Lottery) settle(round int32, drawn Numbers, tickets []Numbers) {
	var count1, count2, count3, count4 int32
	for _, t := range tickets {
		switch n := matches(t, drawn); {
		case n == 5:
			count1++
		case n == 4:
			count2++
		case n == 3:
			count3++
		case n > 0:
			count4++
		}
	}
	prize := l.Status().Prize
	prize1, prize2, prize3, newPrize := l.cfg.Payout(prize, [4]int32{count1, count2, count3, count4})
	draw := Draw{Numbers: drawn, Prize1: prize1, Prize2: prize2, Prize3: prize3}

	l.out.Drawn(round, prize, count1)
	l.write("store drawing", func(ctx context.Context, st Store) error {
		return st.FinishRound(ctx, round, prize, newPrize, draw)
	})
	l.mu.Lock()
	l.draws[round] = draw
	l.last = StoredRound{ID: round, Prize: prize, NewPrize: newPrize, EndDate: l.status.EndDate, Finished: true, Draw: draw}
	l.lastFound = true
	l.status.Round = round + 1
	l.status.Started = false
	l.mu.Unlock()
	l.after(restartDelay.Milliseconds(), l.startRound)
}

// Payout splits jackpot prize among winners, the tickets taking each of the
// four places: each fourth-place ticket gets the fixed prize, and each of
// the first three places shares its rate of what is left among its
// tickets. It returns what one ticket of each of the first three places
// gets, and the next round's jackpot: the base prize plus prize, less those
// three single shares and the fourth place's whole payout. The sums wrap as
// 32-bit integers do.
func (c Config) Payout(prize int32, winners [4]int32) (prize1, prize2, prize3, newPrize int32) {
	prize4 := winners[3] * c.TwoAndOneNumberPrize
	share := func(count int32, rate float64) int32 {
		if count == 0 {
			return 0
		}
		return commons.JavaInt(float64(prize-prize4) * rate / float64(count))
	}
	prize1 = share(winners[0], c.FiveNumberRate)
	prize2 = share(winners[1], c.FourNumberRate)
	prize3 = share(winners[2], c.ThreeNumberRate)
	return prize1, prize2, prize3, c.Prize + prize - (prize1 + prize2 + prize3 + prize4)
}

// after runs step on the queue in delayMillis, at once when it is not
// positive, unless the lottery stops first.
func (l *Lottery) after(delayMillis int64, step func()) {
	l.queue.After(time.Duration(max(delayMillis, 0))*time.Millisecond, func() {
		l.run.Lock()
		defer l.run.Unlock()
		if !l.stopped {
			step()
		}
	})
}

// write queues fn on the lottery's persistence lane, or runs it at once
// without a writer. Each write gets TaskTimeout.
func (l *Lottery) write(what string, fn func(context.Context, Store) error) {
	store, log := l.store, l.log
	job := func() {
		ctx, cancel := context.WithTimeout(context.Background(), TaskTimeout)
		defer cancel()
		if err := fn(ctx, store); err != nil {
			log.Error().Err(err).Msg("lottery: " + what)
		}
	}
	if l.writes == nil {
		job()
		return
	}
	if !l.writes.Enqueue(writeLane, job) {
		log.Error().Msg("lottery: " + what + ": write dropped")
	}
}
