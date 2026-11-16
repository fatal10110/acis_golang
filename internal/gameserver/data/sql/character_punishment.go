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
