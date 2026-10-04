package sql

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"

	"github.com/fatal10110/acis_golang/internal/gameserver/fishchamp"
)

// fishChampionshipEndMemo is the server_memo variable holding the end of
// the running championship week.
const fishChampionshipEndMemo = "fishChampionshipEnd"

// FishingChampionshipStore reads and writes the fishing championship: the
// running week's end in server_memo and its fishers in
// fishing_championship.
type FishingChampionshipStore struct {
	db *sql.DB
}

// NewFishingChampionshipStore returns a FishingChampionshipStore backed by
// db.
func NewFishingChampionshipStore(db *sql.DB) *FishingChampionshipStore {
	return &FishingChampionshipStore{db: db}
}

// Load returns the stored week end, 0 when none is stored, and every
// stored fisher. An end that is not a decimal 64-bit integer is an error.
func (s *FishingChampionshipStore) Load(ctx context.Context) (int64, []fishchamp.Entry, error) {
	var end int64
	var value string
	switch err := s.db.QueryRowContext(ctx, "SELECT value FROM server_memo WHERE var = ?", fishChampionshipEndMemo).Scan(&value); {
	case err == sql.ErrNoRows:
	case err != nil:
		return 0, nil, fmt.Errorf("load fishing championship end: %w", err)
	default:
		if end, err = strconv.ParseInt(value, 10, 64); err != nil {
			return 0, nil, fmt.Errorf("load fishing championship end %q: %w", value, err)
		}
	}
	rows, err := s.db.QueryContext(ctx, "SELECT player_name, fish_length, rewarded FROM fishing_championship")
	if err != nil {
		return 0, nil, fmt.Errorf("load fishing championship: %w", err)
	}
	defer rows.Close()
	var out []fishchamp.Entry
	for rows.Next() {
		var e fishchamp.Entry
		if err := rows.Scan(&e.Name, &e.Length, &e.Reward); err != nil {
			return 0, nil, fmt.Errorf("load fishing championship: %w", err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return 0, nil, fmt.Errorf("load fishing championship: %w", err)
	}
	return end, out, nil
}

// Save stores the week end and replaces the stored fishers with entries,
// in one transaction.
func (s *FishingChampionshipStore) Save(ctx context.Context, end int64, entries []fishchamp.Entry) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("save fishing championship: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx,
		"INSERT INTO server_memo (var, value) VALUES (?, ?) ON DUPLICATE KEY UPDATE value = VALUES(value)",
		fishChampionshipEndMemo, strconv.FormatInt(end, 10),
	); err != nil {
		return fmt.Errorf("save fishing championship end: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM fishing_championship"); err != nil {
		return fmt.Errorf("save fishing championship: %w", err)
	}
	for _, e := range entries {
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO fishing_championship (player_name, fish_length, rewarded) VALUES (?, ?, ?)",
			e.Name, e.Length, int32(e.Reward),
		); err != nil {
			return fmt.Errorf("save fishing championship: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("save fishing championship: %w", err)
	}
	return nil
}
