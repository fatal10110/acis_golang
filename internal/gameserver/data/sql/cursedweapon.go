package sql

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/fatal10110/acis_golang/internal/gameserver/cursedweapon"
)

// CursedWeaponStore reads and writes the held cursed weapons in
// cursed_weapons, and gives a former holder its karma, PK kills and
// stored items back.
type CursedWeaponStore struct {
	db *sql.DB
}

// NewCursedWeaponStore returns a CursedWeaponStore backed by db.
func NewCursedWeaponStore(db *sql.DB) *CursedWeaponStore {
	return &CursedWeaponStore{db: db}
}

// Load returns every row.
func (s *CursedWeaponStore) Load(ctx context.Context) ([]cursedweapon.Row, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT itemId, COALESCE(playerId,0), COALESCE(playerKarma,0), COALESCE(playerPkKills,0),
		COALESCE(nbKills,0), COALESCE(currentStage,0), COALESCE(numberBeforeNextStage,0), COALESCE(hungryTime,0), COALESCE(endTime,0)
		FROM cursed_weapons ORDER BY itemId`)
	if err != nil {
		return nil, fmt.Errorf("load cursed weapons: %w", err)
	}
	defer rows.Close()

	var out []cursedweapon.Row
	for rows.Next() {
		var r cursedweapon.Row
		if err := rows.Scan(&r.ItemID, &r.PlayerID, &r.PlayerKarma, &r.PlayerPKKills, &r.NbKills, &r.CurrentStage, &r.NumberBeforeNextStage, &r.HungryTime, &r.EndTime); err != nil {
			return nil, fmt.Errorf("load cursed weapons: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load cursed weapons: %w", err)
	}
	return out, nil
}

// Insert stores a newly held weapon's row, replacing any stale one.
func (s *CursedWeaponStore) Insert(ctx context.Context, r cursedweapon.Row) error {
	if _, err := s.db.ExecContext(ctx,
		`REPLACE INTO cursed_weapons (itemId, playerId, playerKarma, playerPkKills, nbKills, currentStage, numberBeforeNextStage, hungryTime, endTime)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ItemID, r.PlayerID, r.PlayerKarma, r.PlayerPKKills, r.NbKills, r.CurrentStage, r.NumberBeforeNextStage, r.HungryTime, r.EndTime,
	); err != nil {
		return fmt.Errorf("insert cursed weapon %d: %w", r.ItemID, err)
	}
	return nil
}

// Update stores a held weapon's progress.
func (s *CursedWeaponStore) Update(ctx context.Context, r cursedweapon.Row) error {
	if _, err := s.db.ExecContext(ctx,
		`UPDATE cursed_weapons SET nbKills=?, currentStage=?, numberBeforeNextStage=?, hungryTime=?, endTime=? WHERE itemId=?`,
		r.NbKills, r.CurrentStage, r.NumberBeforeNextStage, r.HungryTime, r.EndTime, r.ItemID,
	); err != nil {
		return fmt.Errorf("update cursed weapon %d: %w", r.ItemID, err)
	}
	return nil
}

// Delete removes a weapon's row.
func (s *CursedWeaponStore) Delete(ctx context.Context, itemID int32) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM cursed_weapons WHERE itemId = ?`, itemID); err != nil {
		return fmt.Errorf("delete cursed weapon %d: %w", itemID, err)
	}
	return nil
}

// ReleaseHolder sets the former holder's karma and PK kills back and, with
// removeItem, deletes the weapon from its stored items.
func (s *CursedWeaponStore) ReleaseHolder(ctx context.Context, playerID, itemID, karma, pkKills int32, removeItem bool) error {
	if removeItem {
		if _, err := s.db.ExecContext(ctx, `DELETE FROM items WHERE owner_id = ? AND item_id = ?`, playerID, itemID); err != nil {
			return fmt.Errorf("remove cursed weapon %d from %d: %w", itemID, playerID, err)
		}
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE characters SET karma = ?, pkkills = ? WHERE obj_Id = ?`, karma, pkKills, playerID); err != nil {
		return fmt.Errorf("restore karma of cursed weapon %d holder %d: %w", itemID, playerID, err)
	}
	return nil
}
