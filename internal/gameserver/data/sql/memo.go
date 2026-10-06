package sql

import (
	"context"
	"database/sql"
	"fmt"
)

// MemoStore reads and writes character_memo rows.
type MemoStore struct {
	db *sql.DB
}

// NewMemoStore returns a MemoStore backed by db.
func NewMemoStore(db *sql.DB) *MemoStore {
	return &MemoStore{db: db}
}

// ListMemos returns ownerID's memos by name.
func (s *MemoStore) ListMemos(ctx context.Context, ownerID int32) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT var,val FROM character_memo WHERE charId=?`, ownerID)
	if err != nil {
		return nil, fmt.Errorf("list memos for owner %d: %w", ownerID, err)
	}
	defer rows.Close()

	out := map[string]string{}
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, fmt.Errorf("list memos for owner %d: %w", ownerID, err)
		}
		out[key] = value
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list memos for owner %d: %w", ownerID, err)
	}
	return out, nil
}

// SetMemo saves value as ownerID's memo key, replacing any saved value.
func (s *MemoStore) SetMemo(ctx context.Context, ownerID int32, key, value string) error {
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO character_memo (charId,var,val) VALUES (?,?,?) ON DUPLICATE KEY UPDATE val=VALUES(val)`,
		ownerID, key, value); err != nil {
		return fmt.Errorf("set memo %s for owner %d: %w", key, ownerID, err)
	}
	return nil
}

// UnsetMemo deletes ownerID's memo key.
func (s *MemoStore) UnsetMemo(ctx context.Context, ownerID int32, key string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM character_memo WHERE charId=? AND var=?`, ownerID, key); err != nil {
		return fmt.Errorf("unset memo %s for owner %d: %w", key, ownerID, err)
	}
	return nil
}
