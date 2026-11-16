package sql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
)

// SpawnStore reads and writes the spawn_data table.
type SpawnStore struct {
	db *sql.DB
}

// NewSpawnStore returns a SpawnStore backed by db.
func NewSpawnStore(db *sql.DB) *SpawnStore {
	return &SpawnStore{db: db}
}

// LoadStates returns every persisted spawn_data row keyed by name.
func (s *SpawnStore) LoadStates(ctx context.Context) (map[string]*spawn.State, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT name, status, current_hp, current_mp, loc_x, loc_y, loc_z, heading, db_value, respawn_time FROM spawn_data ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("load spawn data: %w", err)
	}
	defer rows.Close()

	states := make(map[string]*spawn.State)
	for rows.Next() {
		state := &spawn.State{}
		err := rows.Scan(
			&state.Name,
			&state.Status,
			&state.CurrentHP,
			&state.CurrentMP,
			&state.Location.X,
			&state.Location.Y,
			&state.Location.Z,
			&state.Heading,
			&state.DBValue,
			&state.RespawnTime,
		)
		if err != nil {
			return nil, fmt.Errorf("load spawn data: %w", err)
		}
		states[state.Name] = state
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load spawn data: %w", err)
	}
	return states, nil
}

// spawnColumns lists the spawn_data columns in the order the writes bind
// them.
const spawnColumns = `name, status, current_hp, current_mp, loc_x, loc_y, loc_z, heading, db_value, respawn_time`

// spawnArgs returns state's values in spawnColumns order.
func spawnArgs(state *spawn.State) []any {
	return []any{
		state.Name,
		state.Status,
		state.CurrentHP,
		state.CurrentMP,
		state.Location.X,
		state.Location.Y,
		state.Location.Z,
		state.Heading,
		state.DBValue,
		state.RespawnTime,
	}
}

// SaveStates replaces spawn_data with initialized rows from states. It
// commits nothing when any row fails: the table keeps the rows it had.
func (s *SpawnStore) SaveStates(ctx context.Context, states map[string]*spawn.State) error {
	rows := make([]*spawn.State, 0, len(states))
	for _, state := range states {
		if state == nil || state.Status < 0 {
			continue
		}
		if state.Name == "" {
			return errors.New("save spawn data: empty name")
		}
		rows = append(rows, state)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("save spawn data: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	if _, err := tx.ExecContext(ctx, "DELETE FROM spawn_data"); err != nil {
		return fmt.Errorf("clear spawn data: %w", err)
	}

	stmt, err := tx.PrepareContext(ctx, `INSERT INTO spawn_data (`+spawnColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("save spawn data: %w", err)
	}
	defer stmt.Close()

	for _, state := range rows {
		if _, err := stmt.ExecContext(ctx, spawnArgs(state)...); err != nil {
			return fmt.Errorf("save spawn data %q: %w", state.Name, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("save spawn data: %w", err)
	}
	committed = true
	return nil
}

// SaveState writes the one row state the way SaveStates keeps it: an
// initialized row replaces the saved one, an uninitialized row is removed.
func (s *SpawnStore) SaveState(ctx context.Context, state spawn.State) error {
	if state.Name == "" {
		return errors.New("save spawn data: empty name")
	}
	if state.Status < 0 {
		if _, err := s.db.ExecContext(ctx, "DELETE FROM spawn_data WHERE name = ?", state.Name); err != nil {
			return fmt.Errorf("delete spawn data %q: %w", state.Name, err)
		}
		return nil
	}
	if _, err := s.db.ExecContext(ctx, `REPLACE INTO spawn_data (`+spawnColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, spawnArgs(&state)...); err != nil {
		return fmt.Errorf("save spawn data %q: %w", state.Name, err)
	}
	return nil
}
