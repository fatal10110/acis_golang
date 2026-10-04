package sql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/fatal10110/acis_golang/internal/gameserver/schemebuffer"
)

// BufferSchemeStore reads and writes the players' scheme buffer schemes in
// buffer_schemes.
type BufferSchemeStore struct {
	db *sql.DB
}

// NewBufferSchemeStore returns a BufferSchemeStore backed by db.
func NewBufferSchemeStore(db *sql.DB) *BufferSchemeStore {
	return &BufferSchemeStore{db: db}
}

// Load returns every stored scheme in primary key order: by player, then
// by name.
func (s *BufferSchemeStore) Load(ctx context.Context) ([]schemebuffer.Row, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT object_id, scheme_name, skills FROM buffer_schemes ORDER BY object_id, scheme_name")
	if err != nil {
		return nil, fmt.Errorf("load buffer schemes: %w", err)
	}
	defer rows.Close()
	var out []schemebuffer.Row
	for rows.Next() {
		var row schemebuffer.Row
		var owner int64
		if err := rows.Scan(&owner, &row.Name, &row.Skills); err != nil {
			return nil, fmt.Errorf("load buffer schemes: %w", err)
		}
		row.OwnerID = int32(owner)
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load buffer schemes: %w", err)
	}
	return out, nil
}

// Save replaces every stored scheme with rows in one transaction. A row the
// table refuses (a name its collation reads as a duplicate, or a character
// it cannot hold) is skipped and reported, and every other row is still
// stored. A save that cannot delete, begin or commit keeps the schemes
// stored before it.
func (s *BufferSchemeStore) Save(ctx context.Context, rows []schemebuffer.Row) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("save buffer schemes: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, "DELETE FROM buffer_schemes"); err != nil {
		return fmt.Errorf("save buffer schemes: %w", err)
	}
	var skipped []error
	if len(rows) > 0 {
		stmt, err := tx.PrepareContext(ctx, "INSERT INTO buffer_schemes (object_id, scheme_name, skills) VALUES (?, ?, ?)")
		if err != nil {
			return fmt.Errorf("save buffer schemes: %w", err)
		}
		defer stmt.Close()
		for _, row := range rows {
			if _, err := stmt.ExecContext(ctx, row.OwnerID, row.Name, row.Skills); err != nil {
				if ctx.Err() != nil {
					return fmt.Errorf("save buffer schemes: %w", err)
				}
				skipped = append(skipped, fmt.Errorf("skip buffer scheme %q of %d: %w", row.Name, row.OwnerID, err))
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("save buffer schemes: %w", err)
	}
	return errors.Join(skipped...)
}
