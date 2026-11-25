package sql

import (
	"context"
	"fmt"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// SetPunishment stores level and timer as objectID's punishment.
func (s *CharacterStore) SetPunishment(ctx context.Context, objectID int32, level int, timer int64) error {
	if _, err := s.db.ExecContext(ctx, "UPDATE characters SET punish_level = ?, punish_timer = ? WHERE obj_Id = ?", level, timer, objectID); err != nil {
		return fmt.Errorf("set punishment for %d: %w", objectID, err)
	}
	return nil
}

// SetPunishmentAt stores level and timer as objectID's punishment and moves
// the character to at.
func (s *CharacterStore) SetPunishmentAt(ctx context.Context, objectID int32, level int, timer int64, at location.Location) error {
	if _, err := s.db.ExecContext(ctx, "UPDATE characters SET x = ?, y = ?, z = ?, punish_level = ?, punish_timer = ? WHERE obj_Id = ?", at.X, at.Y, at.Z, level, timer, objectID); err != nil {
		return fmt.Errorf("set punishment for %d: %w", objectID, err)
	}
	return nil
}

// ReplacePunishment stores level and timer as objectID's punishment only
// while the stored one is expected, and returns the level stored before:
// expected when it was replaced. level must differ from expected.
func (s *CharacterStore) ReplacePunishment(ctx context.Context, objectID int32, expected, level int, timer int64) (int, error) {
	res, err := s.db.ExecContext(ctx, "UPDATE characters SET punish_level = ?, punish_timer = ? WHERE obj_Id = ? AND punish_level = ?", level, timer, objectID, expected)
	if err != nil {
		return 0, fmt.Errorf("replace punishment for %d: %w", objectID, err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return 0, fmt.Errorf("replace punishment for %d: %w", objectID, err)
	} else if n > 0 {
		return expected, nil
	}
	var stored int
	if err := s.db.QueryRowContext(ctx, "SELECT punish_level FROM characters WHERE obj_Id = ?", objectID).Scan(&stored); err != nil {
		return 0, fmt.Errorf("read punishment for %d: %w", objectID, err)
	}
	return stored, nil
}
