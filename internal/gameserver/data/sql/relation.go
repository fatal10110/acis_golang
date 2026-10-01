package sql

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"strings"

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

// relationSaveChunk bounds how many rows one multi-row statement of Save
// covers: 3000 placeholders for an upsert, far under the 65535 a prepared
// statement may hold.
const relationSaveChunk = 1000

// Save writes rows back: a row with relation flags is inserted or has its
// flags replaced, a row with none (Relation 0) is deleted. The upserts go
// first, then the deletes, each as multi-row statements of up to
// relationSaveChunk rows. Every statement commits on its own, so the
// statements finished before an error or ctx's deadline stay written.
func (s *RelationStore) Save(ctx context.Context, rows []relation.Row) error {
	var upserts, deletes []relation.Row
	for _, r := range rows {
		if r.Relation == 0 {
			deletes = append(deletes, r)
		} else {
			upserts = append(upserts, r)
		}
	}
	for chunk := range slices.Chunk(upserts, relationSaveChunk) {
		args := make([]any, 0, len(chunk)*3)
		for _, r := range chunk {
			args = append(args, r.CharID, r.FriendID, r.Relation)
		}
		query := `INSERT INTO character_relations (char_id, friend_id, relation) VALUES ` +
			strings.TrimSuffix(strings.Repeat("(?,?,?),", len(chunk)), ",") +
			` ON DUPLICATE KEY UPDATE relation = VALUES(relation)`
		if _, err := s.db.ExecContext(ctx, query, args...); err != nil {
			return fmt.Errorf("save %d character relations (%d-%d..%d-%d): %w",
				len(chunk), chunk[0].CharID, chunk[0].FriendID, chunk[len(chunk)-1].CharID, chunk[len(chunk)-1].FriendID, err)
		}
	}
	for chunk := range slices.Chunk(deletes, relationSaveChunk) {
		args := make([]any, 0, len(chunk)*2)
		for _, r := range chunk {
			args = append(args, r.CharID, r.FriendID)
		}
		query := `DELETE FROM character_relations WHERE (char_id, friend_id) IN (` +
			strings.TrimSuffix(strings.Repeat("(?,?),", len(chunk)), ",") + `)`
		if _, err := s.db.ExecContext(ctx, query, args...); err != nil {
			return fmt.Errorf("delete %d character relations (%d-%d..%d-%d): %w",
				len(chunk), chunk[0].CharID, chunk[0].FriendID, chunk[len(chunk)-1].CharID, chunk[len(chunk)-1].FriendID, err)
		}
	}
	return nil
}
