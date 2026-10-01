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

// SetAccessLevelByName stores level as the access level of the character
// named name, and reports whether a character has that name. A character
// already at level is found too, though its row does not change.
func (s *CharacterStore) SetAccessLevelByName(ctx context.Context, name string, level int) (bool, error) {
	res, err := s.db.ExecContext(ctx, "UPDATE characters SET accesslevel = ? WHERE char_name = ?", level, name)
	if err != nil {
		return false, fmt.Errorf("set access level for %q: %w", name, err)
	}
	changed, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("set access level for %q: %w", name, err)
	}
	if changed > 0 {
		return true, nil
	}
	// The driver counts changed rows, not matched ones.
	var n int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM characters WHERE char_name = ?", name).Scan(&n); err != nil {
		return false, fmt.Errorf("set access level for %q: %w", name, err)
	}
	return n > 0, nil
}
