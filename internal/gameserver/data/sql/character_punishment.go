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

// SetPunishmentByName stores level and timer as the punishment of the
// character named name, and reports whether a character has that name.
func (s *CharacterStore) SetPunishmentByName(ctx context.Context, name string, level int, timer int64) (bool, error) {
	found, err := s.updateByName(ctx, name, "UPDATE characters SET punish_level = ?, punish_timer = ? WHERE char_name = ?", level, timer, name)
	if err != nil {
		return false, fmt.Errorf("set punishment for %q: %w", name, err)
	}
	return found, nil
}

// SetPunishmentAtByName stores level and timer as the punishment of the
// character named name and moves it to at, and reports whether a character
// has that name.
func (s *CharacterStore) SetPunishmentAtByName(ctx context.Context, name string, level int, timer int64, at location.Location) (bool, error) {
	found, err := s.updateByName(ctx, name, "UPDATE characters SET x = ?, y = ?, z = ?, punish_level = ?, punish_timer = ? WHERE char_name = ?", at.X, at.Y, at.Z, level, timer, name)
	if err != nil {
		return false, fmt.Errorf("set punishment for %q: %w", name, err)
	}
	return found, nil
}

// updateByName runs query, an update of the row of the character named
// name, and reports whether a character has that name: a row the update
// left as it was is found too.
func (s *CharacterStore) updateByName(ctx context.Context, name, query string, args ...any) (bool, error) {
	res, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return false, err
	}
	changed, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	if changed > 0 {
		return true, nil
	}
	// The driver counts changed rows, not matched ones.
	var n int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM characters WHERE char_name = ?", name).Scan(&n); err != nil {
		return false, err
	}
	return n > 0, nil
}
