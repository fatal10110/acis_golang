package sql

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/fatal10110/acis_golang/internal/gameserver/social/relation"
)

// RelationStore reads and writes character_relations rows.
type RelationStore struct {
	db *sql.DB
}

// NewRelationStore returns a RelationStore backed by db.
func NewRelationStore(db *sql.DB) *RelationStore {
	return &RelationStore{db: db}
}

// Load returns every character_relations row in primary-key order.
func (s *RelationStore) Load(ctx context.Context) ([]relation.Row, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT char_id, friend_id, relation FROM character_relations ORDER BY char_id, friend_id`)
	if err != nil {
		return nil, fmt.Errorf("load character relations: %w", err)
	}
	defer rows.Close()

	var out []relation.Row
	for rows.Next() {
		var r relation.Row
		if err := rows.Scan(&r.CharID, &r.FriendID, &r.Relation); err != nil {
			return nil, fmt.Errorf("load character relations: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load character relations: %w", err)
	}
	return out, nil
}

// Save writes rows back in one transaction: a row with relation flags is
// inserted or has its flags replaced, a row with none (Relation 0) is
// deleted.
func (s *RelationStore) Save(ctx context.Context, rows []relation.Row) error {
	if len(rows) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("save character relations: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	upsert, err := tx.PrepareContext(ctx,
		`INSERT INTO character_relations (char_id, friend_id, relation) VALUES (?, ?, ?)
		 ON DUPLICATE KEY UPDATE relation = VALUES(relation)`)
	if err != nil {
		return fmt.Errorf("save character relations: %w", err)
	}
	defer upsert.Close()
	del, err := tx.PrepareContext(ctx, `DELETE FROM character_relations WHERE char_id = ? AND friend_id = ?`)
	if err != nil {
		return fmt.Errorf("save character relations: %w", err)
	}
	defer del.Close()

	for _, r := range rows {
		if r.Relation == 0 {
			_, err = del.ExecContext(ctx, r.CharID, r.FriendID)
		} else {
			_, err = upsert.ExecContext(ctx, r.CharID, r.FriendID, r.Relation)
		}
		if err != nil {
			return fmt.Errorf("save character relation %d-%d: %w", r.CharID, r.FriendID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit character relations: %w", err)
	}
	committed = true
	return nil
}
