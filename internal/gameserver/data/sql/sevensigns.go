package sql

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
)

// SevenSignsStore reads and writes the single seven_signs_status row.
type SevenSignsStore struct {
	db *sql.DB
}

// NewSevenSignsStore returns a SevenSignsStore backed by db.
func NewSevenSignsStore(db *sql.DB) *SevenSignsStore {
	return &SevenSignsStore{db: db}
}

// LoadStatus returns the status row id=0, or found=false when the table has
// not been seeded yet.
func (s *SevenSignsStore) LoadStatus(ctx context.Context) (sevensigns.StatusRow, bool, error) {
	var (
		cycle      int
		activeName string
		dateMillis int64
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT current_cycle, active_period, date FROM seven_signs_status WHERE id = 0`,
	).Scan(&cycle, &activeName, &dateMillis)
	if err == sql.ErrNoRows {
		return sevensigns.StatusRow{}, false, nil
	}
	if err != nil {
		return sevensigns.StatusRow{}, false, fmt.Errorf("load seven signs status: %w", err)
	}
	period, err := sevensigns.ParsePeriod(activeName)
	if err != nil {
		return sevensigns.StatusRow{}, false, err
	}
	row := sevensigns.StatusRow{Cycle: cycle, Period: period}
	if dateMillis > 0 {
		row.LastSave = time.UnixMilli(dateMillis)
	}
	return row, true, nil
}

// SaveStatus writes cycle, active period, and save timestamp back to the
// status row id=0, inserting it with schema defaults when it is missing.
func (s *SevenSignsStore) SaveStatus(ctx context.Context, row sevensigns.StatusRow) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE seven_signs_status SET current_cycle = ?, active_period = ?, date = ? WHERE id = 0`,
		row.Cycle, row.Period.String(), row.LastSave.UnixMilli(),
	)
	if err != nil {
		return fmt.Errorf("save seven signs status: %w", err)
	}
	if affected, err := res.RowsAffected(); err == nil && affected > 0 {
		return nil
	}
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO seven_signs_status (id, current_cycle, active_period, date) VALUES (0, ?, ?, ?)`,
		row.Cycle, row.Period.String(), row.LastSave.UnixMilli(),
	); err != nil {
		return fmt.Errorf("insert seven signs status: %w", err)
	}
	return nil
}
