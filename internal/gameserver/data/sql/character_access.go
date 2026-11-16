package sql

import (
	"context"
	"fmt"
)

// SetAccessLevel stores level as objectID's access level, with the title
// the change left it.
func (s *CharacterStore) SetAccessLevel(ctx context.Context, objectID int32, level int, title string) error {
	if _, err := s.db.ExecContext(ctx, "UPDATE characters SET accesslevel = ?, title = ? WHERE obj_Id = ?", level, title, objectID); err != nil {
		return fmt.Errorf("set access level for %d: %w", objectID, err)
	}
	return nil
}

// SetAccessLevelKeepingTitle stores level as objectID's access level,
// leaving its title as it is.
func (s *CharacterStore) SetAccessLevelKeepingTitle(ctx context.Context, objectID int32, level int) error {
	if _, err := s.db.ExecContext(ctx, "UPDATE characters SET accesslevel = ? WHERE obj_Id = ?", level, objectID); err != nil {
		return fmt.Errorf("set access level for %d: %w", objectID, err)
	}
	return nil
}
