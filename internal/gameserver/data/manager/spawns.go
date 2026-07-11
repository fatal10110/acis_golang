package manager

import (
	"context"
	"fmt"

	"github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
)

type spawnStateStore interface {
	LoadStates(ctx context.Context) (map[string]*spawn.State, error)
	SaveStates(ctx context.Context, states map[string]*spawn.State) error
}

// Spawns combines the static spawnlist table with dynamic DB-backed state.
type Spawns struct {
	table  *spawn.Table
	states map[string]*spawn.State
}

// LoadSpawns loads spawnlist XML from dir, restores dynamic rows from store,
// and creates uninitialized rows for XML dbName entries missing in the DB.
func LoadSpawns(ctx context.Context, dir string, store spawnStateStore) (*Spawns, error) {
	table, err := xml.LoadSpawnlist(dir)
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

// Save persists initialized dynamic state rows through store.
func (s *Spawns) Save(ctx context.Context, store spawnStateStore) error {
	return store.SaveStates(ctx, s.states)
}
