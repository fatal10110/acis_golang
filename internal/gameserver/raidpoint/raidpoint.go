// Package raidpoint owns the raid points players earn by killing raid and
// grand bosses: each player's points per boss, the ranking built from their
// totals, and their persistence in character_raid_points.
//
// The monthly reset that turns the top 100 players' points into clan
// reputation and wipes them all is a scheduled job of the script engine
// (#172).
package raidpoint

import (
	"context"
	"sync"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/rnd"
	"github.com/rs/zerolog"
)

// taskTimeout bounds one database write.
const taskTimeout = 10 * time.Second

// writeLane is the persistence owner every raid point write is queued
// under: one lane keeps the writes in the order the points changed, so a
// later total never lands before an earlier one. No character has object
// id 0.
const writeLane int32 = 0

// Row is one stored character_raid_points row.
type Row struct {
	CharID, BossID, Points int32
}

// Entry is one boss in a player's record.
type Entry struct {
	BossID, Points int32
}

// Store persists the points.
type Store interface {
	// Load returns every stored row, ordered by character then boss.
	Load(ctx context.Context) ([]Row, error)
	// Save stores the player's total for one boss, replacing any earlier
	// one.
	Save(ctx context.Context, row Row) error
}

// Writer runs a database write later, on ownerID's persistence lane.
type Writer interface {
	Enqueue(ownerID int32, job func()) bool
}

// Points holds every player's raid points. It is safe for concurrent use.
type Points struct {
	store  Store
	writes Writer
	log    zerolog.Logger

	// mu guards records; a write is queued while it is held, so the
	// queue order is the order the points changed in.
	mu      sync.Mutex
	records map[int32]*record
}

// New returns empty raid points persisting through store, their writes
// queued on writes (run inline when nil). Restore loads the stored ones.
func New(store Store, writes Writer, log zerolog.Logger) *Points {
	return &Points{store: store, writes: writes, log: log, records: map[int32]*record{}}
}

// Restore loads the stored points.
func (p *Points) Restore(ctx context.Context) error {
	rows, err := p.store.Load(ctx)
	if err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, row := range rows {
		p.recordOf(row.CharID).restore(row.BossID, row.Points)
	}
	p.log.Info().Int("players", len(p.records)).Msg("raid points: restored")
	return nil
}

// killPoints rolls the points one player earns for a kill of a boss of
// the given level: half the level, give or take five.
func killPoints(bossLevel int) int32 {
	return int32(bossLevel/2 + rnd.GetRange(-5, 5))
}

// CreditKill gives each player the points of a kill of the boss, each
// rolling its own.
func (p *Points) CreditKill(objectIDs []int32, bossID int32, bossLevel int) {
	for _, id := range objectIDs {
		p.Add(id, bossID, killPoints(bossLevel))
	}
}

// Add adds points to the player's points for the boss and stores the new
// total. A negative amount adds nothing; zero still enters the boss in the
// player's record.
func (p *Points) Add(objectID, bossID, points int32) {
	if points < 0 {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	total := p.recordOf(objectID).add(bossID, points)
	row := Row{CharID: objectID, BossID: bossID, Points: total}
	p.writeLocked("save", func(ctx context.Context, st Store) error { return st.Save(ctx, row) })
}

// Record is one player's raid point record.
type Record struct {
	// Rank is the player's place in the ranking, 0 when it holds no
	// points.
	Rank int32
	// Total is the sum of its points.
	Total int32
	// Entries is its points per boss, in the order the record lists them.
	Entries []Entry
	// Found is false when the player has no record at all.
	Found bool
}

// Record returns the player's record.
func (p *Points) Record(objectID int32) Record {
	p.mu.Lock()
	defer p.mu.Unlock()
	r := p.records[objectID]
	if r == nil {
		return Record{}
	}
	out := Record{Total: r.total(), Entries: r.entries(), Found: true}
	if out.Total > 0 {
		out.Rank = 1 + p.aheadLocked(objectID, out.Total)
	}
	return out
}

// aheadLocked counts the players ranked before objectID, whose total is
// total: those with more points, and those with as many and a lower object
// id.
func (p *Points) aheadLocked(objectID, total int32) int32 {
	var n int32
	for id, r := range p.records {
		if t := r.total(); t > total || (t == total && id < objectID) {
			n++
		}
	}
	return n
}

func (p *Points) recordOf(objectID int32) *record {
	r := p.records[objectID]
	if r == nil {
		r = &record{points: map[int32]int32{}}
		p.records[objectID] = r
	}
	return r
}

// writeLocked queues fn on the raid points' persistence lane while p.mu is
// held, or runs it at once without a writer. Each write gets taskTimeout.
func (p *Points) writeLocked(what string, fn func(context.Context, Store) error) {
	store, log := p.store, p.log
	job := func() {
		ctx, cancel := context.WithTimeout(context.Background(), taskTimeout)
		defer cancel()
		if err := fn(ctx, store); err != nil {
			log.Error().Err(err).Msg("raid points: " + what)
		}
	}
	if p.writes == nil {
		job()
		return
	}
	if !p.writes.Enqueue(writeLane, job) {
		log.Error().Msg("raid points: " + what + ": write dropped")
	}
}
