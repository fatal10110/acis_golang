package sql

import (
	"context"
	"database/sql"
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

// SaveStates replaces spawn_data with initialized rows from states.
func (s *SpawnStore) SaveStates(ctx context.Context, states map[string]*spawn.State) error {
	if _, err := s.db.ExecContext(ctx, "TRUNCATE spawn_data"); err != nil {
		return fmt.Errorf("clear spawn data: %w", err)
	}

	stmt, err := s.db.PrepareContext(ctx, `INSERT INTO spawn_data (name, status, current_hp, current_mp, loc_x, loc_y, loc_z, heading, db_value, respawn_time) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("save spawn data: %w", err)
	}
	defer stmt.Close()

	for _, state := range states {
		if state == nil || state.Status < 0 {
			continue
		}
		if state.Name == "" {
			return fmt.Errorf("save spawn data: empty name")
		}
		if _, err := stmt.ExecContext(ctx,
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
		); err != nil {
			return fmt.Errorf("save spawn data %q: %w", state.Name, err)
		}
	}
	return nil
}
