package sql

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/shortcut"
)

const activeShortcutClassIndex = 0

// ShortcutStore reads and writes character_shortcuts rows.
type ShortcutStore struct {
	db *sql.DB
}

// NewShortcutStore returns a ShortcutStore backed by db.
func NewShortcutStore(db *sql.DB) *ShortcutStore {
	return &ShortcutStore{db: db}
}

// ListByOwner returns ownerID's shortcuts for the active class slot.
func (s *ShortcutStore) ListByOwner(ctx context.Context, ownerID int32) ([]shortcut.Shortcut, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT slot, page, type, id, level
		 FROM character_shortcuts WHERE char_obj_id = ? AND class_index = ?
		 ORDER BY page, slot`,
		ownerID, activeShortcutClassIndex,
	)
	if err != nil {
		return nil, fmt.Errorf("list shortcuts for owner %d: %w", ownerID, err)
	}
	defer rows.Close()

	var out []shortcut.Shortcut
	for rows.Next() {
		var sc shortcut.Shortcut
		var typ string
		if err := rows.Scan(&sc.Slot, &sc.Page, &typ, &sc.ID, &sc.Level); err != nil {
			return nil, fmt.Errorf("list shortcuts for owner %d: %w", ownerID, err)
		}
		parsed, ok := shortcut.ParseType(typ)
		if !ok {
			return nil, fmt.Errorf("list shortcuts for owner %d: unknown type %q", ownerID, typ)
		}
		sc.Type = parsed
		sc.CharacterType = 1
		out = append(out, sc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list shortcuts for owner %d: %w", ownerID, err)
	}
	return out, nil
}

// Save inserts or replaces sc for ownerID's active class slot.
func (s *ShortcutStore) Save(ctx context.Context, ownerID int32, sc shortcut.Shortcut) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO character_shortcuts
			(char_obj_id, slot, page, type, id, level, class_index)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE type=VALUES(type), id=VALUES(id), level=VALUES(level)`,
		ownerID, sc.Slot, sc.Page, sc.Type.String(), sc.ID, sc.Level, activeShortcutClassIndex,
	)
	if err != nil {
		return fmt.Errorf("save shortcut owner %d slot %d page %d: %w", ownerID, sc.Slot, sc.Page, err)
	}
	return nil
}

// Delete removes one shortcut from ownerID's active class slot.
func (s *ShortcutStore) Delete(ctx context.Context, ownerID int32, slot, page int32) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM character_shortcuts
		 WHERE char_obj_id = ? AND slot = ? AND page = ? AND class_index = ?`,
		ownerID, slot, page, activeShortcutClassIndex,
	)
	if err != nil {
		return fmt.Errorf("delete shortcut owner %d slot %d page %d: %w", ownerID, slot, page, err)
	}
	return nil
}

// DeleteByOwner removes every shortcut owned by ownerID.
func (s *ShortcutStore) DeleteByOwner(ctx context.Context, ownerID int32) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM character_shortcuts WHERE char_obj_id = ?`, ownerID)
	if err != nil {
		return fmt.Errorf("delete shortcuts for owner %d: %w", ownerID, err)
	}
	return nil
}
