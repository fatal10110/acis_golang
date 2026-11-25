package sql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"

	"github.com/fatal10110/acis_golang/internal/gameserver/castlemanor"
)

// manorLastRolloverMemo is the server_memo variable holding when the manor
// last rolled its period over, in Unix milliseconds.
const manorLastRolloverMemo = "manorLastRollover"

// ManorStore reads and writes the castles' manor lists in
// castle_manor_production and castle_manor_procure, and the last rollover's
// time in server_memo.
type ManorStore struct {
	db *sql.DB
}

// NewManorStore returns a ManorStore backed by db.
func NewManorStore(db *sql.DB) *ManorStore {
	return &ManorStore{db: db}
}

var _ castlemanor.Store = (*ManorStore)(nil)

// Load returns every stored row, each table ordered by its primary key:
// castle, item, then period, and the last rollover's time, 0 when none is
// stored. A time that is not a decimal 64-bit integer is an error.
func (s *ManorStore) Load(ctx context.Context) ([]castlemanor.ProductionRow, []castlemanor.ProcureRow, int64, error) {
	production, err := queryRows(ctx, s.db,
		"SELECT castle_id, seed_id, amount, start_amount, price, next_period FROM castle_manor_production ORDER BY castle_id, seed_id, next_period",
		func(rows *sql.Rows) (castlemanor.ProductionRow, error) {
			var r castlemanor.ProductionRow
			err := rows.Scan(&r.CastleID, &r.SeedID, &r.Amount, &r.StartAmount, &r.Price, &r.NextPeriod)
			return r, err
		})
	if err != nil {
		return nil, nil, 0, fmt.Errorf("load manor production: %w", err)
	}
	procure, err := queryRows(ctx, s.db,
		"SELECT castle_id, crop_id, amount, start_amount, price, reward_type, next_period FROM castle_manor_procure ORDER BY castle_id, crop_id, next_period",
		func(rows *sql.Rows) (castlemanor.ProcureRow, error) {
			var r castlemanor.ProcureRow
			err := rows.Scan(&r.CastleID, &r.CropID, &r.Amount, &r.StartAmount, &r.Price, &r.RewardType, &r.NextPeriod)
			return r, err
		})
	if err != nil {
		return nil, nil, 0, fmt.Errorf("load manor procure: %w", err)
	}
	var lastRollover int64
	var value string
	switch err := s.db.QueryRowContext(ctx, "SELECT value FROM server_memo WHERE var = ?", manorLastRolloverMemo).Scan(&value); {
	case errors.Is(err, sql.ErrNoRows):
	case err != nil:
		return nil, nil, 0, fmt.Errorf("load manor last rollover: %w", err)
	default:
		if lastRollover, err = strconv.ParseInt(value, 10, 64); err != nil {
			return nil, nil, 0, fmt.Errorf("load manor last rollover %q: %w", value, err)
		}
	}
	return production, procure, lastRollover, nil
}

// Save replaces every stored row with production and procure, and stores
// lastRollover unless it is 0, in one transaction.
func (s *ManorStore) Save(ctx context.Context, production []castlemanor.ProductionRow, procure []castlemanor.ProcureRow, lastRollover int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("save manor: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, "DELETE FROM castle_manor_production"); err != nil {
		return fmt.Errorf("save manor production: %w", err)
	}
	for _, r := range production {
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO castle_manor_production (castle_id, seed_id, amount, start_amount, price, next_period) VALUES (?, ?, ?, ?, ?, ?)",
			r.CastleID, r.SeedID, r.Amount, r.StartAmount, r.Price, r.NextPeriod,
		); err != nil {
			return fmt.Errorf("save manor production: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM castle_manor_procure"); err != nil {
		return fmt.Errorf("save manor procure: %w", err)
	}
	for _, r := range procure {
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO castle_manor_procure (castle_id, crop_id, amount, start_amount, price, reward_type, next_period) VALUES (?, ?, ?, ?, ?, ?, ?)",
			r.CastleID, r.CropID, r.Amount, r.StartAmount, r.Price, r.RewardType, r.NextPeriod,
		); err != nil {
			return fmt.Errorf("save manor procure: %w", err)
		}
	}
	if lastRollover != 0 {
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO server_memo (var, value) VALUES (?, ?) ON DUPLICATE KEY UPDATE value = VALUES(value)",
			manorLastRolloverMemo, strconv.FormatInt(lastRollover, 10),
		); err != nil {
			return fmt.Errorf("save manor last rollover: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("save manor: %w", err)
	}
	return nil
}
