package manager

import (
	"context"
	"fmt"
	"sync"

	"github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/rs/zerolog"
)

type spawnStateStore interface {
	LoadStates(ctx context.Context) (map[string]*spawn.State, error)
	SaveStates(ctx context.Context, states map[string]*spawn.State) error
}

// Spawns combines the static spawnlist table with dynamic DB-backed state.
type Spawns struct {
	table *spawn.Table
	// mu guards the rows' DBValue: scripts set it from the NPCs' queues
	// while a save may be reading the rows.
	mu     sync.Mutex
	states map[string]*spawn.State
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

// State returns dynamic state by DB name.
func (s *Spawns) State(name string) (*spawn.State, bool) {
	state, ok := s.states[name]
	return state, ok
}

// States returns a shallow copy of all dynamic state rows keyed by DB name.
func (s *Spawns) States() map[string]*spawn.State {
	out := make(map[string]*spawn.State, len(s.states))
	for name, state := range s.states {
		out[name] = state
	}
	return out
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
	state, ok := s.states[name]
	if !ok || state == nil {
		return 0, false
	}
	return state.DBValue, true
}

// SetDBValue stores v as the script value of the row name, saved with the
// row; nothing when the spawn list has no such row.
func (s *Spawns) SetDBValue(name string, v int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if state, ok := s.states[name]; ok && state != nil {
		state.DBValue = v
	}
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
