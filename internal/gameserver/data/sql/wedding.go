package sql

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/fatal10110/acis_golang/internal/gameserver/wedding"
)

// CoupleStore reads and writes the couples in mods_wedding.
type CoupleStore struct {
	db *sql.DB
}

// NewCoupleStore returns a CoupleStore backed by db.
func NewCoupleStore(db *sql.DB) *CoupleStore {
	return &CoupleStore{db: db}
}

// Load returns every stored couple, by id.
func (s *CoupleStore) Load(ctx context.Context) ([]wedding.Couple, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, requesterId, partnerId FROM mods_wedding ORDER BY id")
	if err != nil {
		return nil, fmt.Errorf("load couples: %w", err)
	}
	defer rows.Close()
	var out []wedding.Couple
	for rows.Next() {
		var id, requester, partner int64
		if err := rows.Scan(&id, &requester, &partner); err != nil {
			return nil, fmt.Errorf("load couples: %w", err)
		}
		out = append(out, wedding.Couple{ID: int32(id), RequesterID: int32(requester), PartnerID: int32(partner)})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load couples: %w", err)
	}
	return out, nil
}

// Save replaces every stored couple with couples in one transaction. A
// save that fails keeps the couples stored before it.
func (s *CoupleStore) Save(ctx context.Context, couples []wedding.Couple) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("save couples: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, "DELETE FROM mods_wedding"); err != nil {
		return fmt.Errorf("save couples: %w", err)
	}
	if len(couples) > 0 {
		stmt, err := tx.PrepareContext(ctx, "INSERT INTO mods_wedding (id, requesterId, partnerId) VALUES (?, ?, ?)")
		if err != nil {
			return fmt.Errorf("save couples: %w", err)
		}
		defer stmt.Close()
		for _, c := range couples {
			if _, err := stmt.ExecContext(ctx, c.ID, c.RequesterID, c.PartnerID); err != nil {
				return fmt.Errorf("save couple %d: %w", c.ID, err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("save couples: %w", err)
	}
	return nil
}
