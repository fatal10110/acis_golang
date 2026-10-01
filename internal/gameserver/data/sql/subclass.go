package sql

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
)

// SubclassStore reads and writes character_subclasses rows, and clears the
// rows a replaced subclass leaves in the per-class tables.
type SubclassStore struct {
	db *sql.DB
}

// NewSubclassStore returns a SubclassStore backed by db.
func NewSubclassStore(db *sql.DB) *SubclassStore {
	return &SubclassStore{db: db}
}

// List returns charID's subclasses by ascending class index.
func (s *SubclassStore) List(ctx context.Context, charID int32) ([]player.SubClass, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT class_id, exp, sp, level, class_index FROM character_subclasses
		 WHERE char_obj_id = ? ORDER BY class_index ASC`, charID)
	if err != nil {
		return nil, fmt.Errorf("list subclasses for character %d: %w", charID, err)
	}
	defer rows.Close()
	var out []player.SubClass
	for rows.Next() {
		var sub player.SubClass
		if err := rows.Scan(&sub.ClassID, &sub.Exp, &sub.SP, &sub.Level, &sub.Index); err != nil {
			return nil, fmt.Errorf("list subclasses for character %d: %w", charID, err)
		}
		out = append(out, sub)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list subclasses for character %d: %w", charID, err)
	}
	return out, nil
}

// Insert writes sub as a new subclass row of charID.
func (s *SubclassStore) Insert(ctx context.Context, charID int32, sub player.SubClass) error {
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO character_subclasses (char_obj_id, class_id, exp, sp, level, class_index) VALUES (?, ?, ?, ?, ?, ?)`,
		charID, sub.ClassID, sub.Exp, sub.SP, sub.Level, sub.Index); err != nil {
		return fmt.Errorf("insert subclass %d for character %d: %w", sub.Index, charID, err)
	}
	return nil
}

// Delete removes everything charID keeps for class index: its hennas,
// shortcuts, saved effects, learned skills and the subclass row itself,
// in that order. A failure stops at the table it failed on.
func (s *SubclassStore) Delete(ctx context.Context, charID int32, index int) error {
	for _, table := range []string{"character_hennas", "character_shortcuts", "character_skills_save", "character_skills", "character_subclasses"} {
		if _, err := s.db.ExecContext(ctx, "DELETE FROM "+table+" WHERE char_obj_id = ? AND class_index = ?", charID, index); err != nil {
			return fmt.Errorf("delete %s of subclass %d for character %d: %w", table, index, charID, err)
		}
	}
	return nil
}

// updateSubclasses writes each subclass's progression and class back to
// its row of charID.
func updateSubclasses(ctx context.Context, db *sql.DB, charID int32, subs []player.SubClass) error {
	for _, sub := range subs {
		if _, err := db.ExecContext(ctx,
			`UPDATE character_subclasses SET exp = ?, sp = ?, level = ?, class_id = ? WHERE char_obj_id = ? AND class_index = ?`,
			sub.Exp, sub.SP, sub.Level, sub.ClassID, charID, sub.Index); err != nil {
			return fmt.Errorf("save subclass %d for character %d: %w", sub.Index, charID, err)
		}
	}
	return nil
}
