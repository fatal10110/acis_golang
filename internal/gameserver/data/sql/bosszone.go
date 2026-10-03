package sql

import (
	"context"
	"database/sql"
	"fmt"
	"maps"
	"slices"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
)

// BossZoneStore reads and writes the players allowed into each boss zone
// across a restart, kept in grandboss_list.
type BossZoneStore struct {
	db *sql.DB
}

// NewBossZoneStore returns a BossZoneStore backed by db.
func NewBossZoneStore(db *sql.DB) *BossZoneStore {
	return &BossZoneStore{db: db}
}

// Load returns the stored players of every zone, keyed by zone id.
func (s *BossZoneStore) Load(ctx context.Context) (map[int][]int32, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT player_id, zone FROM grandboss_list ORDER BY zone, player_id")
	if err != nil {
		return nil, fmt.Errorf("load boss zone players: %w", err)
	}
	defer rows.Close()

	out := map[int][]int32{}
	for rows.Next() {
		var playerID, zoneID int64
		if err := rows.Scan(&playerID, &zoneID); err != nil {
			return nil, fmt.Errorf("load boss zone players: %w", err)
		}
		out[int(zoneID)] = append(out[int(zoneID)], int32(playerID))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load boss zone players: %w", err)
	}
	return out, nil
}

// Save replaces every stored row with players, keyed by zone id.
func (s *BossZoneStore) Save(ctx context.Context, players map[int][]int32) error {
	if _, err := s.db.ExecContext(ctx, "TRUNCATE grandboss_list"); err != nil {
		return fmt.Errorf("clear boss zone players: %w", err)
	}
	for _, zoneID := range slices.Sorted(maps.Keys(players)) {
		for _, playerID := range players[zoneID] {
			if _, err := s.db.ExecContext(ctx, "INSERT INTO grandboss_list (player_id, zone) VALUES (?, ?)", playerID, zoneID); err != nil {
				return fmt.Errorf("save boss zone %d player %d: %w", zoneID, playerID, err)
			}
		}
	}
	return nil
}

// Restore gives each stored player of a zone in zones its entry permission
// back. The permission's re-entry deadline is the moment of the restore, so
// it already lapsed when the player next enters: what survives is the
// permission itself, which lets the player's summon in and is saved again
// at the next shutdown. Rows of a zone not in zones are ignored.
func (s *BossZoneStore) Restore(ctx context.Context, zones []*zone.Boss) error {
	players, err := s.Load(ctx)
	if err != nil {
		return err
	}
	for _, z := range zones {
		for _, id := range players[z.ID()] {
			z.AllowEntry(id, 0)
		}
	}
	return nil
}

// SaveZones replaces every stored row with the players currently allowed
// into each zone in zones.
func (s *BossZoneStore) SaveZones(ctx context.Context, zones []*zone.Boss) error {
	players := map[int][]int32{}
	for _, z := range zones {
		if ids := z.AllowedPlayers(); len(ids) > 0 {
			slices.Sort(ids)
			players[z.ID()] = append(players[z.ID()], ids...)
		}
	}
	return s.Save(ctx, players)
}
