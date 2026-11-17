package manager

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/fatal10110/acis_golang/internal/gameserver/persist"
	"github.com/rs/zerolog"
)

type spawnStateStore interface {
	LoadStates(ctx context.Context) (map[string]*spawn.State, error)
	SaveStates(ctx context.Context, states map[string]*spawn.State) error
}

// spawnRowStore writes one spawn_data row, or the whole list in one go.
type spawnRowStore interface {
	spawnStateStore
	SaveState(ctx context.Context, state spawn.State) error
}

// spawnRowQueue runs the row writes off the callers' goroutines.
type spawnRowQueue interface {
	Enqueue(ownerID int32, job func()) bool
	Flush(ctx context.Context, ownerIDs ...int32) error
}

// spawnWriteLane is the persistence owner every row write is queued under,
// so the writes of one row land in the order they were made, and no
// player's saves wait behind them.
const spawnWriteLane = persist.ServiceOwner

// SpawnRowWriteTimeout bounds one row write.
const SpawnRowWriteTimeout = 10 * time.Second

// rowWrites writes a row on the persistence queue as soon as its death,
// respawn deadline or db value changes, so a stop without the shutdown save
// keeps it.
type rowWrites struct {
	store spawnRowStore
	queue spawnRowQueue
	log   zerolog.Logger
}

// Spawns combines the static spawnlist table with dynamic DB-backed state.
type Spawns struct {
	table *spawn.Table
	// mu guards every field of every row: decays, respawns and scripts
	// write them from the NPCs' queues while a save may be reading them.
	// It is a leaf lock: no other lock is taken under it.
	mu     sync.Mutex
	states map[string]*spawn.State
	// writes is nil when rows are saved only as a whole.
	writes *rowWrites
}

// LoadSpawns loads spawnlist XML from dir, restores dynamic rows from store,
// and creates uninitialized rows for XML dbName entries missing in the DB.
// spawnMultiplier is the npcs.properties SpawnMultiplier setting.
func LoadSpawns(ctx context.Context, dir string, store spawnStateStore, log zerolog.Logger, spawnMultiplier float64) (*Spawns, error) {
	table, err := xml.LoadSpawnlist(dir, log, spawnMultiplier)
	if err != nil {
		return nil, err
	}

	states := map[string]*spawn.State{}
	if store != nil {
		states, err = store.LoadStates(ctx)
		if err != nil {
			return nil, fmt.Errorf("load spawn states: %w", err)
		}
	}
	return NewSpawns(table, states), nil
}

// NewSpawns returns a Spawns view over table and states.
func NewSpawns(table *spawn.Table, states map[string]*spawn.State) *Spawns {
	copied := make(map[string]*spawn.State, len(states))
	for name, state := range states {
		if state != nil && state.Name == "" {
			state.Name = name
		}
		copied[name] = state
	}

	if table != nil {
		for _, maker := range table.Makers() {
			for _, entry := range maker.Entries {
				if entry.DBName == "" {
					continue
				}
				if _, ok := copied[entry.DBName]; !ok {
					copied[entry.DBName] = spawn.NewState(entry.DBName)
				}
			}
		}
	}

	return &Spawns{table: table, states: copied}
}

// Table returns the static spawnlist table.
func (s *Spawns) Table() *spawn.Table {
	return s.table
}

// State returns a copy of the row name.
func (s *Spawns) State(name string) (spawn.State, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.states[name]
	if !ok || state == nil {
		return spawn.State{}, false
	}
	return *state, true
}

// StateCount returns the number of dynamic state rows.
func (s *Spawns) StateCount() int {
	return len(s.states)
}

// DBValue returns the script value of the row name; false when the spawn
// list has no such row.
func (s *Spawns) DBValue(name string) (int, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.row(name)
	if state == nil {
		return 0, false
	}
	return state.DBValue, true
}

// SetDBValue stores v as the script value of the row name, written with the
// row when it changes; nothing when the spawn list has no such row.
func (s *Spawns) SetDBValue(name string, v int) {
	s.mu.Lock()
	state := s.row(name)
	changed := state != nil && state.DBValue != v
	if changed {
		state.DBValue = v
	}
	s.mu.Unlock()
	if changed {
		s.write(name)
	}
}

// row returns the row name; nil when the spawn list has none. The caller
// holds s.mu.
func (s *Spawns) row(name string) *spawn.State {
	return s.states[name]
}

// RespawnDeadline returns when the row name, saved dead, respawns, and
// whether it still waits for that deadline at now.
func (s *Spawns) RespawnDeadline(name string, now time.Time) (time.Time, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.row(name)
	if state == nil || !state.Dead(now) {
		return time.Time{}, false
	}
	return time.UnixMilli(state.RespawnTime), true
}

// Restore applies the placement restore rule to the row name for a spawn at
// (loc, heading) with maxHP and maxMP: it returns the row and true when the
// row was alive, its saved HP, MP and position to reuse; false after it
// makes the row a fresh live one. A spawn list with no such row reports
// false.
func (s *Spawns) Restore(name string, loc location.Location, heading, maxHP, maxMP int, now time.Time) (spawn.State, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.row(name)
	if state == nil || !state.CheckAlive(loc, heading, maxHP, maxMP, now) {
		return spawn.State{}, false
	}
	return *state, true
}

// SetStats stores the live HP, MP and position of the row name; a dead row
// keeps its respawn deadline.
func (s *Spawns) SetStats(name string, hp, mp int, loc location.Location, heading int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if state := s.row(name); state != nil {
		state.SetStats(hp, mp, loc, heading)
	}
}

// SetRespawn saves the row name dead, respawning delay after now.
func (s *Spawns) SetRespawn(name string, delay time.Duration, now time.Time) {
	s.update(name, func(state *spawn.State) { state.SetRespawn(delay, now) })
}

// CancelRespawn clears the respawn deadline of the row name.
func (s *Spawns) CancelRespawn(name string) {
	s.update(name, (*spawn.State).CancelRespawn)
}

// Uninitialize marks the row name not spawned yet: it is not saved until a
// placement makes it live again.
func (s *Spawns) Uninitialize(name string) {
	s.update(name, func(state *spawn.State) { state.Status = spawn.StatusUninitialized })
}

// uninitializeTracked marks every database-tracked row of the spawn list not
// spawned yet, writing nothing; writeAll then lands them in one save.
func (s *Spawns) uninitializeTracked() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.table == nil {
		return
	}
	for _, maker := range s.table.Makers() {
		for _, entry := range maker.Entries {
			if entry.DBName == "" {
				continue
			}
			if state := s.row(entry.DBName); state != nil {
				state.Status = spawn.StatusUninitialized
			}
		}
	}
}

// update applies change to the row name, then writes the row; nothing when
// the spawn list has no such row.
func (s *Spawns) update(name string, change func(*spawn.State)) {
	s.mu.Lock()
	state := s.row(name)
	if state != nil {
		change(state)
	}
	s.mu.Unlock()
	if state != nil {
		s.write(name)
	}
}

// WriteRows makes every later death, respawn deadline and db value change
// write its row through store on queue (owner-approved divergence, #4685:
// otherwise rows reach the database only with a whole save).
func (s *Spawns) WriteRows(store spawnRowStore, queue spawnRowQueue, log zerolog.Logger) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.writes = &rowWrites{store: store, queue: queue, log: log}
}

// writeRowsLike makes s write its rows the way from does.
func (s *Spawns) writeRowsLike(from *Spawns) {
	if from == nil || from == s {
		return
	}
	from.mu.Lock()
	writes := from.writes
	from.mu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.writes = writes
}

// write queues a write of the row name. The job reads the row when it runs,
// so a write never lands older than the one queued before it. The caller
// holds no lock: a queue with no worker runs the job inline.
func (s *Spawns) write(name string) {
	s.mu.Lock()
	w := s.writes
	s.mu.Unlock()
	if w == nil {
		return
	}
	w.queue.Enqueue(spawnWriteLane, func() {
		row, ok := s.State(name)
		if !ok {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), SpawnRowWriteTimeout)
		defer cancel()
		if err := w.store.SaveState(ctx, row); err != nil {
			w.log.Error().Err(err).Str("spawn", name).Msg("save spawn data row")
		}
	})
}

// writeAll queues one whole-list save of the rows, read when it runs, so a
// change to every row costs one transaction rather than a write per row. The
// caller holds no lock: a queue with no worker runs the job inline.
func (s *Spawns) writeAll() {
	s.mu.Lock()
	w := s.writes
	s.mu.Unlock()
	if w == nil {
		return
	}
	w.queue.Enqueue(spawnWriteLane, func() {
		ctx, cancel := context.WithTimeout(context.Background(), SpawnRowWriteTimeout)
		defer cancel()
		if err := s.Save(ctx, w.store); err != nil {
			w.log.Error().Err(err).Msg("save spawn data")
		}
	})
}

// FlushRowWrites waits until every row write queued so far has landed, or
// ctx ends.
func (s *Spawns) FlushRowWrites(ctx context.Context) error {
	s.mu.Lock()
	w := s.writes
	s.mu.Unlock()
	if w == nil {
		return nil
	}
	return w.queue.Flush(ctx, spawnWriteLane)
}

// Save persists initialized dynamic state rows through store. The rows are
// copied first, so the database writes run with no lock held.
func (s *Spawns) Save(ctx context.Context, store spawnStateStore) error {
	s.mu.Lock()
	rows := make(map[string]*spawn.State, len(s.states))
	for name, state := range s.states {
		if state == nil {
			continue
		}
		row := *state
		rows[name] = &row
	}
	s.mu.Unlock()
	return store.SaveStates(ctx, rows)
}
