package sql

import (
	"context"
	"fmt"
)

// SetNoble stores objectID's noblesse status.
func (s *CharacterStore) SetNoble(ctx context.Context, objectID int32, noble bool) error {
	if _, err := s.db.ExecContext(ctx, "UPDATE characters SET nobless = ? WHERE obj_Id = ?", noble, objectID); err != nil {
		return fmt.Errorf("set noble for %d: %w", objectID, err)
	}
	return nil
}

// SetTitle stores objectID's title.
func (s *CharacterStore) SetTitle(ctx context.Context, objectID int32, title string) error {
	if _, err := s.db.ExecContext(ctx, "UPDATE characters SET title = ? WHERE obj_Id = ?", title, objectID); err != nil {
		return fmt.Errorf("set title for %d: %w", objectID, err)
	}
	return nil
}

// SetClanPenalties stores the ends, in epoch milliseconds, of objectID's
// clan join and clan creation penalties.
func (s *CharacterStore) SetClanPenalties(ctx context.Context, objectID int32, joinExpiry, createExpiry int64) error {
	if _, err := s.db.ExecContext(ctx, "UPDATE characters SET clan_join_expiry_time = ?, clan_create_expiry_time = ? WHERE obj_Id = ?", joinExpiry, createExpiry, objectID); err != nil {
		return fmt.Errorf("set clan penalties for %d: %w", objectID, err)
	}
	return nil
}
