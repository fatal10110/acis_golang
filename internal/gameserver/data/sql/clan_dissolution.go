package sql

import (
	"context"
	"fmt"
)

// DeleteClan deletes a dissolved clan's rows in one transaction: its
// clan_data, clan_privs, clan_skills and clan_subpledges rows, every
// clan_wars row naming it on either side and its siege_clans
// registrations, and resets the tax of castleID, the castle it held, when
// it held one.
func (s *ClanStore) DeleteClan(ctx context.Context, clanID, castleID int32) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("delete clan %d: %w", clanID, err)
	}
	defer func() { _ = tx.Rollback() }()
	exec := func(query string, args ...any) {
		if err == nil {
			_, err = tx.ExecContext(ctx, query, args...)
		}
	}
	exec(`DELETE FROM clan_data WHERE clan_id=?`, clanID)
	exec(`DELETE FROM clan_privs WHERE clan_id=?`, clanID)
	exec(`DELETE FROM clan_skills WHERE clan_id=?`, clanID)
	exec(`DELETE FROM clan_subpledges WHERE clan_id=?`, clanID)
	exec(`DELETE FROM clan_wars WHERE clan1=? OR clan2=?`, clanID, clanID)
	exec(`DELETE FROM siege_clans WHERE clan_id=?`, clanID)
	if castleID != 0 {
		exec(`UPDATE castle SET currentTaxPercent=0, nextTaxPercent=0 WHERE id=?`, castleID)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		return fmt.Errorf("delete clan %d: %w", clanID, err)
	}
	return nil
}
