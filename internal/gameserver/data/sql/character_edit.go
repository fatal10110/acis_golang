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

// SetSex stores objectID's sex.
func (s *CharacterStore) SetSex(ctx context.Context, objectID int32, sex byte) error {
	if _, err := s.db.ExecContext(ctx, "UPDATE characters SET sex = ? WHERE obj_Id = ?", sex, objectID); err != nil {
		return fmt.Errorf("set sex for %d: %w", objectID, err)
	}
	return nil
}

// SetName stores objectID's name.
func (s *CharacterStore) SetName(ctx context.Context, objectID int32, name string) error {
	if _, err := s.db.ExecContext(ctx, "UPDATE characters SET char_name = ? WHERE obj_Id = ?", name, objectID); err != nil {
		return fmt.Errorf("set name for %d: %w", objectID, err)
	}
	return nil
}
