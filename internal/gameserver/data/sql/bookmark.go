package sql

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/bookmark"
)

// BookmarkStore reads and writes bookmarks rows.
type BookmarkStore struct {
	db *sql.DB
}

// NewBookmarkStore returns a BookmarkStore backed by db.
func NewBookmarkStore(db *sql.DB) *BookmarkStore {
	return &BookmarkStore{db: db}
}

// ListByOwner returns ownerID's bookmarks in key order: by name, as the
// table's collation sorts it. A NULL coordinate reads as 0.
func (s *BookmarkStore) ListByOwner(ctx context.Context, ownerID int32) ([]bookmark.Bookmark, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT name, COALESCE(x, 0), COALESCE(y, 0), COALESCE(z, 0) FROM bookmarks WHERE obj_Id = ? ORDER BY name",
		ownerID,
	)
	if err != nil {
		return nil, fmt.Errorf("list bookmarks of %d: %w", ownerID, err)
	}
	defer rows.Close()

	var out []bookmark.Bookmark
	for rows.Next() {
		var m bookmark.Bookmark
		if err := rows.Scan(&m.Name, &m.X, &m.Y, &m.Z); err != nil {
			return nil, fmt.Errorf("list bookmarks of %d: %w", ownerID, err)
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list bookmarks of %d: %w", ownerID, err)
	}
	return out, nil
}

// Insert adds m for ownerID.
func (s *BookmarkStore) Insert(ctx context.Context, ownerID int32, m bookmark.Bookmark) error {
	if _, err := s.db.ExecContext(ctx,
		"INSERT INTO bookmarks (name, obj_Id, x, y, z) VALUES (?, ?, ?, ?, ?)",
		m.Name, ownerID, m.X, m.Y, m.Z,
	); err != nil {
		return fmt.Errorf("insert bookmark %q of %d: %w", m.Name, ownerID, err)
	}
	return nil
}

// Delete removes ownerID's bookmark called name, as the table's collation
// matches it.
func (s *BookmarkStore) Delete(ctx context.Context, ownerID int32, name string) error {
	if _, err := s.db.ExecContext(ctx, "DELETE FROM bookmarks WHERE name = ? AND obj_Id = ?", name, ownerID); err != nil {
		return fmt.Errorf("delete bookmark %q of %d: %w", name, ownerID, err)
	}
	return nil
}
