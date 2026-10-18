package sql

import (
	"context"
	"fmt"

	"github.com/fatal10110/acis_golang/internal/gameserver/clanhall"
)

var _ clanhall.SiegeStore = (*ClanHallStore)(nil)

// UpdateSiegableHall writes a siegable hall's owner and next siege date,
// kept in endDate.
func (s *ClanHallStore) UpdateSiegableHall(ctx context.Context, hallID, ownerID int32, nextSiege int64) error {
	if _, err := s.db.ExecContext(ctx, "UPDATE clanhall SET ownerId = ?, endDate = ? WHERE id = ?", ownerID, nextSiege, hallID); err != nil {
		return fmt.Errorf("store siegable hall %d: %w", hallID, err)
	}
	return nil
}

// LoadSiegeAttackers returns every clanhall_siege_attackers row.
func (s *ClanHallStore) LoadSiegeAttackers(ctx context.Context) ([]clanhall.AttackerRow, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT clanhall_id, attacker_id FROM clanhall_siege_attackers")
	if err != nil {
		return nil, fmt.Errorf("load hall siege attackers: %w", err)
	}
	var out []clanhall.AttackerRow
	for rows.Next() {
		var r clanhall.AttackerRow
		if err := rows.Scan(&r.HallID, &r.ClanID); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan hall siege attacker: %w", err)
		}
		out = append(out, r)
	}
	if err := closeRows(rows); err != nil {
		return nil, fmt.Errorf("load hall siege attackers: %w", err)
	}
	return out, nil
}

// SaveSiegeAttackers replaces hall hallID's attackers with clanIDs, in one
// transaction.
func (s *ClanHallStore) SaveSiegeAttackers(ctx context.Context, hallID int32, clanIDs []int32) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store hall %d siege attackers: begin: %w", hallID, err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, "DELETE FROM clanhall_siege_attackers WHERE clanhall_id = ?", hallID); err != nil {
		return fmt.Errorf("delete hall %d siege attackers: %w", hallID, err)
	}
	for _, clanID := range clanIDs {
		if _, err := tx.ExecContext(ctx, "INSERT INTO clanhall_siege_attackers VALUES (?, ?)", hallID, clanID); err != nil {
			return fmt.Errorf("insert hall %d siege attacker %d: %w", hallID, clanID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store hall %d siege attackers: commit: %w", hallID, err)
	}
	return nil
}

// LoadRainbowDecrees returns every rainbowsprings_attacker_list row.
func (s *ClanHallStore) LoadRainbowDecrees(ctx context.Context) ([]clanhall.DecreeRow, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT clanId, war_decrees_count FROM rainbowsprings_attacker_list")
	if err != nil {
		return nil, fmt.Errorf("load rainbow springs decrees: %w", err)
	}
	var out []clanhall.DecreeRow
	for rows.Next() {
		var r clanhall.DecreeRow
		if err := rows.Scan(&r.ClanID, &r.Count); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan rainbow springs decree: %w", err)
		}
		out = append(out, r)
	}
	if err := closeRows(rows); err != nil {
		return nil, fmt.Errorf("load rainbow springs decrees: %w", err)
	}
	return out, nil
}

// AddRainbowDecree inserts r.
func (s *ClanHallStore) AddRainbowDecree(ctx context.Context, r clanhall.DecreeRow) error {
	if _, err := s.db.ExecContext(ctx, "INSERT INTO rainbowsprings_attacker_list VALUES (?, ?)", r.ClanID, r.Count); err != nil {
		return fmt.Errorf("store rainbow springs decree of clan %d: %w", r.ClanID, err)
	}
	return nil
}

// DeleteRainbowDecree deletes clan clanID's decrees.
func (s *ClanHallStore) DeleteRainbowDecree(ctx context.Context, clanID int32) error {
	if _, err := s.db.ExecContext(ctx, "DELETE FROM rainbowsprings_attacker_list WHERE clanId = ?", clanID); err != nil {
		return fmt.Errorf("delete rainbow springs decree of clan %d: %w", clanID, err)
	}
	return nil
}

// DeleteRainbowDecrees deletes every clan's decrees.
func (s *ClanHallStore) DeleteRainbowDecrees(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, "DELETE FROM rainbowsprings_attacker_list"); err != nil {
		return fmt.Errorf("delete rainbow springs decrees: %w", err)
	}
	return nil
}
