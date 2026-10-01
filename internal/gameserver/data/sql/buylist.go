package sql

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/merchant"
)

// BuyListStore reads and writes the buylists rows: the saved count and
// restock time of each limited product sold since its last restock.
type BuyListStore struct {
	db *sql.DB
}

// NewBuyListStore returns a BuyListStore backed by db.
func NewBuyListStore(db *sql.DB) *BuyListStore {
	return &BuyListStore{db: db}
}

// LoadStock returns every buylists row.
func (s *BuyListStore) LoadStock(ctx context.Context) ([]merchant.StockRow, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT buylist_id, item_id, count, next_restock_time FROM buylists`)
	if err != nil {
		return nil, fmt.Errorf("load buylists: %w", err)
	}
	defer rows.Close()

	var out []merchant.StockRow
	for rows.Next() {
		var row merchant.StockRow
		var next int64
		if err := rows.Scan(&row.ListID, &row.ItemID, &row.Count, &next); err != nil {
			return nil, fmt.Errorf("load buylists: %w", err)
		}
		row.NextRestock = time.UnixMilli(next)
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load buylists: %w", err)
	}
	return out, nil
}

// SaveStock writes row, replacing the product's earlier row.
func (s *BuyListStore) SaveStock(ctx context.Context, row merchant.StockRow) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO buylists (buylist_id, item_id, count, next_restock_time) VALUES (?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE count = VALUES(count), next_restock_time = VALUES(next_restock_time)`,
		row.ListID, row.ItemID, row.Count, row.NextRestock.UnixMilli(),
	)
	if err != nil {
		return fmt.Errorf("save buylist %d item %d: %w", row.ListID, row.ItemID, err)
	}
	return nil
}

// DeleteStock deletes the row of listID's product itemID.
func (s *BuyListStore) DeleteStock(ctx context.Context, listID int, itemID int32) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM buylists WHERE buylist_id = ? AND item_id = ?`, listID, itemID); err != nil {
		return fmt.Errorf("delete buylist %d item %d: %w", listID, itemID, err)
	}
	return nil
}
