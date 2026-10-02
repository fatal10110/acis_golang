package sql

import (
	"context"
	"fmt"
)

// UpdateNotice stores a clan's community board notice and whether it shows
// at login.
func (s *ClanStore) UpdateNotice(ctx context.Context, clanID int32, enabled bool, notice string) error {
	if _, err := s.db.ExecContext(ctx, `UPDATE clan_data SET enabled=?,notice=? WHERE clan_id=?`, enabled, notice, clanID); err != nil {
		return fmt.Errorf("update clan %d notice: %w", clanID, err)
	}
	return nil
}

// UpdateIntroduction stores a clan's community board introduction.
func (s *ClanStore) UpdateIntroduction(ctx context.Context, clanID int32, introduction string) error {
	if _, err := s.db.ExecContext(ctx, `UPDATE clan_data SET introduction=? WHERE clan_id=?`, introduction, clanID); err != nil {
		return fmt.Errorf("update clan %d introduction: %w", clanID, err)
	}
	return nil
}
