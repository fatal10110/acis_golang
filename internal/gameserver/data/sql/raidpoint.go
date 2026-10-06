package sql

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/fatal10110/acis_golang/internal/gameserver/raidpoint"
)

// RaidPointStore reads and writes the raid points in character_raid_points.
type RaidPointStore struct {
	db *sql.DB
}

// NewRaidPointStore returns a RaidPointStore backed by db.
func NewRaidPointStore(db *sql.DB) *RaidPointStore {
	return &RaidPointStore{db: db}
}

// Load returns every row, ordered by character then boss.
func (s *RaidPointStore) Load(ctx context.Context) ([]raidpoint.Row, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT char_id, boss_id, points FROM character_raid_points ORDER BY char_id, boss_id")
	if err != nil {
		return nil, fmt.Errorf("load raid points: %w", err)
	}
	defer rows.Close()

	var out []raidpoint.Row
	for rows.Next() {
		var charID, bossID, points int64
		if err := rows.Scan(&charID, &bossID, &points); err != nil {
			return nil, fmt.Errorf("load raid points: %w", err)
		}
		out = append(out, raidpoint.Row{CharID: int32(charID), BossID: int32(bossID), Points: int32(points)})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load raid points: %w", err)
	}
	return out, nil
}

// Save stores the character's total for one boss, replacing any earlier
// one.
func (s *RaidPointStore) Save(ctx context.Context, row raidpoint.Row) error {
	if _, err := s.db.ExecContext(ctx,
		"REPLACE INTO character_raid_points (char_id, boss_id, points) VALUES (?, ?, ?)",
		row.CharID, row.BossID, row.Points,
	); err != nil {
		return fmt.Errorf("save raid points of %d for boss %d: %w", row.CharID, row.BossID, err)
	}
	return nil
}

// Clear removes every row.
func (s *RaidPointStore) Clear(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, "TRUNCATE character_raid_points"); err != nil {
		return fmt.Errorf("clear raid points: %w", err)
	}
	return nil
}
