package sql

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/macro"
)

// MacroStore reads and writes character_macroses rows.
type MacroStore struct {
	db *sql.DB
}

// NewMacroStore returns a MacroStore backed by db.
func NewMacroStore(db *sql.DB) *MacroStore {
	return &MacroStore{db: db}
}

// ListByOwner returns ownerID's macro rows in id order. A NULL text column
// reads as empty; a NULL command column is reported through CommandsValid.
func (s *MacroStore) ListByOwner(ctx context.Context, ownerID int32) ([]macro.Row, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, COALESCE(icon, 0), COALESCE(name, ''), COALESCE(descr, ''), COALESCE(acronym, ''), commands
		 FROM character_macroses WHERE char_obj_id = ? ORDER BY id`,
		ownerID,
	)
	if err != nil {
		return nil, fmt.Errorf("list macros for owner %d: %w", ownerID, err)
	}
	defer rows.Close()

	var out []macro.Row
	for rows.Next() {
		var row macro.Row
		var commands sql.NullString
		if err := rows.Scan(&row.ID, &row.Icon, &row.Name, &row.Description, &row.Acronym, &commands); err != nil {
			return nil, fmt.Errorf("list macros for owner %d: %w", ownerID, err)
		}
		row.Commands, row.CommandsValid = commands.String, commands.Valid
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list macros for owner %d: %w", ownerID, err)
	}
	return out, nil
}

// Save inserts m for ownerID, or updates the row with its id.
func (s *MacroStore) Save(ctx context.Context, ownerID int32, m macro.Macro) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO character_macroses (char_obj_id, id, icon, name, descr, acronym, commands)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE icon=VALUES(icon), name=VALUES(name), descr=VALUES(descr), acronym=VALUES(acronym), commands=VALUES(commands)`,
		ownerID, m.ID, m.Icon, m.Name, m.Description, m.Acronym, macro.EncodeCommands(m.Commands),
	)
	if err != nil {
		return fmt.Errorf("save macro %d for owner %d: %w", m.ID, ownerID, err)
	}
	return nil
}

// Delete removes ownerID's macro id.
func (s *MacroStore) Delete(ctx context.Context, ownerID, id int32) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM character_macroses WHERE char_obj_id = ? AND id = ?`, ownerID, id); err != nil {
		return fmt.Errorf("delete macro %d for owner %d: %w", id, ownerID, err)
	}
	return nil
}
