package sql

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// ItemStore reads and writes the items table.
type ItemStore struct {
	db *sql.DB
}

// NewItemStore returns an ItemStore backed by db.
func NewItemStore(db *sql.DB) *ItemStore {
	return &ItemStore{db: db}
}

// Create inserts inst as a new items row owned by ownerID.
func (s *ItemStore) Create(ctx context.Context, ownerID int32, inst item.Instance) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO items
			(owner_id, object_id, item_id, count, enchant_level, loc, loc_data, custom_type1, custom_type2, mana_left, time)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		ownerID, inst.ObjectID, inst.TemplateID, inst.Count, inst.EnchantLevel,
		inst.Location.String(), inst.LocationData, inst.CustomType1, inst.CustomType2, inst.ManaLeft, inst.Time,
	)
	if err != nil {
		return fmt.Errorf("create item %d for owner %d: %w", inst.ObjectID, ownerID, err)
	}
	return nil
}

// Save inserts or updates inst in the items table.
func (s *ItemStore) Save(ctx context.Context, inst *item.Instance) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO items
			(owner_id, object_id, item_id, count, enchant_level, loc, loc_data, custom_type1, custom_type2, mana_left, time)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?)
		 ON DUPLICATE KEY UPDATE
			owner_id=VALUES(owner_id), count=VALUES(count), loc=VALUES(loc), loc_data=VALUES(loc_data),
			enchant_level=VALUES(enchant_level), custom_type1=VALUES(custom_type1), custom_type2=VALUES(custom_type2),
			mana_left=VALUES(mana_left), time=VALUES(time)`,
		inst.OwnerID, inst.ObjectID, inst.TemplateID, inst.Count, inst.EnchantLevel,
		inst.Location.String(), inst.LocationData, inst.CustomType1, inst.CustomType2, inst.ManaLeft, inst.Time,
	)
	if err != nil {
		return fmt.Errorf("save item %d: %w", inst.ObjectID, err)
	}
	return nil
}

// ListByOwner returns every item ownerID owns, in no particular order.
func (s *ItemStore) ListByOwner(ctx context.Context, ownerID int32) ([]*item.Instance, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT object_id, item_id, count, enchant_level, loc, loc_data, custom_type1, custom_type2, mana_left, time
		 FROM items WHERE owner_id = ?`, ownerID)
	if err != nil {
		return nil, fmt.Errorf("list items for owner %d: %w", ownerID, err)
	}
	defer rows.Close()

	out, err := scanItems(rows, ownerID)
	if err != nil {
		return nil, fmt.Errorf("list items for owner %d: %w", ownerID, err)
	}
	return out, nil
}

// ListByOwnerAndLocations returns every item ownerID owns sitting at one of
// locs, ordered by loc_data — the shape a container restores itself from: a
// plain container passes its one base location, an equip-capable inventory
// passes its base and equip locations together so paperdoll items come
// back ordered by paperdoll position.
func (s *ItemStore) ListByOwnerAndLocations(ctx context.Context, ownerID int32, locs ...item.Location) ([]*item.Instance, error) {
	if len(locs) == 0 {
		return nil, nil
	}

	placeholders := strings.Repeat("?,", len(locs))
	placeholders = placeholders[:len(placeholders)-1]

	args := make([]any, 0, len(locs)+1)
	args = append(args, ownerID)
	for _, loc := range locs {
		args = append(args, loc.String())
	}

	rows, err := s.db.QueryContext(ctx,
		fmt.Sprintf(`SELECT object_id, item_id, count, enchant_level, loc, loc_data, custom_type1, custom_type2, mana_left, time
		 FROM items WHERE owner_id = ? AND loc IN (%s) ORDER BY loc_data`, placeholders),
		args...)
	if err != nil {
		return nil, fmt.Errorf("list items for owner %d: %w", ownerID, err)
	}
	defer rows.Close()

	out, err := scanItems(rows, ownerID)
	if err != nil {
		return nil, fmt.Errorf("list items for owner %d: %w", ownerID, err)
	}
	return out, nil
}

func scanItems(rows *sql.Rows, ownerID int32) ([]*item.Instance, error) {
	out := []*item.Instance{}
	for rows.Next() {
		var inst item.Instance
		var loc string
		inst.OwnerID = ownerID

		if err := rows.Scan(&inst.ObjectID, &inst.TemplateID, &inst.Count, &inst.EnchantLevel,
			&loc, &inst.LocationData, &inst.CustomType1, &inst.CustomType2, &inst.ManaLeft, &inst.Time); err != nil {
			return nil, err
		}
		parsed, err := item.ParseLocation(loc)
		if err != nil {
			return nil, err
		}
		inst.Location = parsed
		out = append(out, &inst)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// Update overwrites the persisted state of inst's row.
func (s *ItemStore) Update(ctx context.Context, inst *item.Instance) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE items SET owner_id=?, item_id=?, count=?, enchant_level=?, loc=?, loc_data=?,
			custom_type1=?, custom_type2=?, mana_left=?, time=?
		 WHERE object_id=?`,
		inst.OwnerID, inst.TemplateID, inst.Count, inst.EnchantLevel, inst.Location.String(), inst.LocationData,
		inst.CustomType1, inst.CustomType2, inst.ManaLeft, inst.Time, inst.ObjectID,
	)
	if err != nil {
		return fmt.Errorf("update item %d: %w", inst.ObjectID, err)
	}
	return nil
}

// Delete removes the items row identified by objectID, if any.
func (s *ItemStore) Delete(ctx context.Context, objectID int32) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM items WHERE object_id = ?", objectID)
	if err != nil {
		return fmt.Errorf("delete item %d: %w", objectID, err)
	}
	return nil
}

// DeleteByOwner removes every items row owned by ownerID and reports how
// many rows were deleted.
func (s *ItemStore) DeleteByOwner(ctx context.Context, ownerID int32) (int64, error) {
	res, err := s.db.ExecContext(ctx, "DELETE FROM items WHERE owner_id = ?", ownerID)
	if err != nil {
		return 0, fmt.Errorf("delete items for owner %d: %w", ownerID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("delete items for owner %d: %w", ownerID, err)
	}
	return n, nil
}
