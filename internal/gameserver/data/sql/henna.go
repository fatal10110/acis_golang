package sql

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/henna"
)

const activeHennaClassIndex = 0

// HennaStore reads and writes character_hennas rows.
type HennaStore struct {
	db *sql.DB
}

// NewHennaStore returns a HennaStore backed by db.
func NewHennaStore(db *sql.DB) *HennaStore {
	return &HennaStore{db: db}
}

// ListByOwner returns ownerID's henna rows for the active class slot,
// ordered by DB slot.
func (s *HennaStore) ListByOwner(ctx context.Context, ownerID int32) ([]henna.Row, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT slot, symbol_id FROM character_hennas
		 WHERE char_obj_id = ? AND class_index = ?
		 ORDER BY slot`,
		ownerID, activeHennaClassIndex,
	)
	if err != nil {
		return nil, fmt.Errorf("list hennas for owner %d: %w", ownerID, err)
	}
	defer rows.Close()

	var out []henna.Row
	for rows.Next() {
		var row henna.Row
		if err := rows.Scan(&row.Slot, &row.SymbolID); err != nil {
			return nil, fmt.Errorf("list hennas for owner %d: %w", ownerID, err)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list hennas for owner %d: %w", ownerID, err)
	}
	return out, nil
}

// Insert writes one equipped dye for ownerID's active class slot.
func (s *HennaStore) Insert(ctx context.Context, ownerID int32, symbolID, slot int) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO character_hennas (char_obj_id, symbol_id, slot, class_index)
		 VALUES (?, ?, ?, ?)`,
		ownerID, symbolID, slot, activeHennaClassIndex,
	)
	if err != nil {
		return fmt.Errorf("insert henna owner %d slot %d: %w", ownerID, slot, err)
	}
	return nil
}

// Delete removes one equipped dye slot from ownerID's active class slot.
func (s *HennaStore) Delete(ctx context.Context, ownerID int32, slot int) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM character_hennas
		 WHERE char_obj_id = ? AND slot = ? AND class_index = ?`,
		ownerID, slot, activeHennaClassIndex,
	)
	if err != nil {
		return fmt.Errorf("delete henna owner %d slot %d: %w", ownerID, slot, err)
	}
	return nil
}

// DeleteByOwner removes every henna row owned by ownerID.
func (s *HennaStore) DeleteByOwner(ctx context.Context, ownerID int32) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM character_hennas WHERE char_obj_id = ?`, ownerID)
	if err != nil {
		return fmt.Errorf("delete hennas for owner %d: %w", ownerID, err)
	}
	return nil
}
