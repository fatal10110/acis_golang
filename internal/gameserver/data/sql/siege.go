package sql

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/fatal10110/acis_golang/internal/gameserver/siege"
)

// SiegeStore reads and writes the siege_clans table: the clans registered
// on each castle's siege, and their side.
type SiegeStore struct {
	db *sql.DB
}

// NewSiegeStore returns a SiegeStore backed by db.
func NewSiegeStore(db *sql.DB) *SiegeStore {
	return &SiegeStore{db: db}
}

// LoadClans returns every siege_clans row, by castle then clan id. A row
// whose type names no side is skipped.
func (s *SiegeStore) LoadClans(ctx context.Context) ([]siege.ClanRow, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT castle_id, clan_id, type FROM siege_clans ORDER BY castle_id, clan_id`)
	if err != nil {
		return nil, fmt.Errorf("load siege clans: %w", err)
	}
	var out []siege.ClanRow
	for rows.Next() {
		var r siege.ClanRow
		var side sql.NullString
		if err := rows.Scan(&r.CastleID, &r.ClanID, &side); err != nil {
			rows.Close()
			return nil, fmt.Errorf("load siege clans: %w", err)
		}
		var ok bool
		if r.Side, ok = siege.ParseSide(side.String); !ok {
			continue
		}
		out = append(out, r)
	}
	if err := closeRows(rows); err != nil {
		return nil, fmt.Errorf("load siege clans: %w", err)
	}
	return out, nil
}

func (s *SiegeStore) exec(ctx context.Context, what, query string, args ...any) error {
	if _, err := s.db.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("%s: %w", what, err)
	}
	return nil
}

// SaveClan registers clanID on castleID's siege with side, replacing the
// side of an earlier registration.
func (s *SiegeStore) SaveClan(ctx context.Context, castleID, clanID int32, side siege.Side) error {
	return s.exec(ctx, "save siege clan",
		`INSERT INTO siege_clans (clan_id, castle_id, type) VALUES (?, ?, ?) ON DUPLICATE KEY UPDATE type=VALUES(type)`,
		clanID, castleID, side.String())
}

// DeleteClan drops clanID's registration on castleID's siege.
func (s *SiegeStore) DeleteClan(ctx context.Context, castleID, clanID int32) error {
	return s.exec(ctx, "delete siege clan", `DELETE FROM siege_clans WHERE castle_id=? AND clan_id=?`, castleID, clanID)
}

// DeleteClans drops every registration on castleID's siege.
func (s *SiegeStore) DeleteClans(ctx context.Context, castleID int32) error {
	return s.exec(ctx, "delete siege clans", `DELETE FROM siege_clans WHERE castle_id=?`, castleID)
}

// DeletePending drops the defender requests still waiting on castleID's
// siege.
func (s *SiegeStore) DeletePending(ctx context.Context, castleID int32) error {
	return s.exec(ctx, "delete pending siege clans", `DELETE FROM siege_clans WHERE castle_id=? AND type='PENDING'`, castleID)
}
