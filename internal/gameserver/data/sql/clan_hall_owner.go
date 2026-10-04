package sql

import (
	"context"
	"fmt"

	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
)

// LoadHallOwners reads the owner of every owned clanhall row, by hall id.
func (s *ClanStore) LoadHallOwners(ctx context.Context) ([]clan.HallOwner, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, ownerId FROM clanhall WHERE ownerId > 0 ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("load clan hall owners: %w", err)
	}
	var owners []clan.HallOwner
	for rows.Next() {
		var o clan.HallOwner
		if err := rows.Scan(&o.HallID, &o.ClanID); err != nil {
			rows.Close()
			return nil, fmt.Errorf("load clan hall owners: %w", err)
		}
		owners = append(owners, o)
	}
	if err := closeRows(rows); err != nil {
		return nil, fmt.Errorf("load clan hall owners: %w", err)
	}
	return owners, nil
}
