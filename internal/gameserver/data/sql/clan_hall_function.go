package sql

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/fatal10110/acis_golang/internal/gameserver/clanhall"
)

// ClanHallFunctionStore reads and writes the clanhall_functions rows.
type ClanHallFunctionStore struct {
	db *sql.DB
}

// NewClanHallFunctionStore returns a ClanHallFunctionStore backed by db.
func NewClanHallFunctionStore(db *sql.DB) *ClanHallFunctionStore {
	return &ClanHallFunctionStore{db: db}
}

// LoadFunctions returns every stored function, by hall and type.
func (s *ClanHallFunctionStore) LoadFunctions(ctx context.Context) ([]clanhall.StoredFunction, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT hall_id, type, lvl, lease, rate, endTime FROM clanhall_functions ORDER BY hall_id, type")
	if err != nil {
		return nil, fmt.Errorf("load clan hall functions: %w", err)
	}
	var out []clanhall.StoredFunction
	for rows.Next() {
		var f clanhall.StoredFunction
		if err := rows.Scan(&f.HallID, &f.Type, &f.Level, &f.Lease, &f.Rate, &f.EndTime); err != nil {
			rows.Close()
			return nil, fmt.Errorf("load clan hall functions: %w", err)
		}
		out = append(out, f)
	}
	if err := closeRows(rows); err != nil {
		return nil, fmt.Errorf("load clan hall functions: %w", err)
	}
	return out, nil
}

// SaveFunction stores f for hall hallID, replacing its row of the same
// type.
func (s *ClanHallFunctionStore) SaveFunction(ctx context.Context, hallID int32, f clanhall.Function) error {
	if _, err := s.db.ExecContext(ctx,
		"REPLACE INTO clanhall_functions (hall_id, type, lvl, lease, rate, endTime) VALUES (?, ?, ?, ?, ?, ?)",
		hallID, f.Type, f.Level, f.Lease, f.Rate, f.EndTime,
	); err != nil {
		return fmt.Errorf("store clan hall %d function %d: %w", hallID, f.Type, err)
	}
	return nil
}

// DeleteFunction drops hall hallID's function of type funcType.
func (s *ClanHallFunctionStore) DeleteFunction(ctx context.Context, hallID int32, funcType int) error {
	if _, err := s.db.ExecContext(ctx, "DELETE FROM clanhall_functions WHERE hall_id = ? AND type = ?", hallID, funcType); err != nil {
		return fmt.Errorf("remove clan hall %d function %d: %w", hallID, funcType, err)
	}
	return nil
}
