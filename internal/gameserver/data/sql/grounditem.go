package sql

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// GroundItemStore reads and writes the items_on_ground table.
type GroundItemStore struct {
	db *sql.DB
}

// NewGroundItemStore returns a GroundItemStore backed by db.
func NewGroundItemStore(db *sql.DB) *GroundItemStore {
	return &GroundItemStore{db: db}
}

// LoadAndClear returns every persisted ground item, then clears the table.
func (s *GroundItemStore) LoadAndClear(ctx context.Context) ([]item.GroundSnapshot, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("load ground items: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	rows, err := tx.QueryContext(ctx, `SELECT object_id,item_id,count,enchant_level,x,y,z,time FROM items_on_ground ORDER BY object_id`)
	if err != nil {
		return nil, fmt.Errorf("load ground items: %w", err)
	}
	out, err := scanGroundItems(rows)
	if err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("load ground items: %w", err)
	}

	if _, err := tx.ExecContext(ctx, "DELETE FROM items_on_ground"); err != nil {
		return nil, fmt.Errorf("clear ground items: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("load ground items: %w", err)
	}
	committed = true
	return out, nil
}

// Save replaces items_on_ground with rows.
func (s *GroundItemStore) Save(ctx context.Context, rows []item.GroundSnapshot) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("save ground items: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	if _, err := tx.ExecContext(ctx, "DELETE FROM items_on_ground"); err != nil {
		return fmt.Errorf("clear ground items: %w", err)
	}

	stmt, err := tx.PrepareContext(ctx, `INSERT INTO items_on_ground(object_id,item_id,count,enchant_level,x,y,z,time) VALUES(?,?,?,?,?,?,?,?)`)
	if err != nil {
		return fmt.Errorf("save ground items: %w", err)
	}
	defer stmt.Close()

	for _, row := range rows {
		if _, err := stmt.ExecContext(ctx,
			row.ObjectID,
			row.TemplateID,
			row.Count,
			row.EnchantLevel,
			row.X,
			row.Y,
			row.Z,
			row.TimeLeftMillis,
		); err != nil {
			return fmt.Errorf("save ground item %d: %w", row.ObjectID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("save ground items: %w", err)
	}
	committed = true
	return nil
}

func scanGroundItems(rows *sql.Rows) ([]item.GroundSnapshot, error) {
	var out []item.GroundSnapshot
	for rows.Next() {
		var row item.GroundSnapshot
		var itemID, count, enchant, x, y, z, millis sql.NullInt64
		if err := rows.Scan(&row.ObjectID, &itemID, &count, &enchant, &x, &y, &z, &millis); err != nil {
			return nil, fmt.Errorf("load ground items: %w", err)
		}
		row.TemplateID = int32(nullInt64(itemID))
		row.Count = int(nullInt64(count))
		row.EnchantLevel = int(nullInt64(enchant))
		row.X = int(nullInt64(x))
		row.Y = int(nullInt64(y))
		row.Z = int(nullInt64(z))
		row.TimeLeftMillis = nullInt64(millis)
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load ground items: %w", err)
	}
	return out, nil
}

func nullInt64(n sql.NullInt64) int64 {
	if !n.Valid {
		return 0
	}
	return n.Int64
}
