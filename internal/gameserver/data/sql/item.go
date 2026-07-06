package sql

import (
	"database/sql"
	"fmt"

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
func (s *ItemStore) Create(ownerID int32, inst item.Instance) error {
	_, err := s.db.Exec(
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

// ListByOwner returns every item ownerID owns, in no particular order.
func (s *ItemStore) ListByOwner(ownerID int32) ([]*item.Instance, error) {
	rows, err := s.db.Query(
		`SELECT object_id, item_id, count, enchant_level, loc, loc_data, custom_type1, custom_type2, mana_left, time
		 FROM items WHERE owner_id = ?`, ownerID)
	if err != nil {
		return nil, fmt.Errorf("list items for owner %d: %w", ownerID, err)
	}
	defer rows.Close()

	out := []*item.Instance{}
	for rows.Next() {
		var inst item.Instance
		var loc string
		inst.OwnerID = ownerID

		if err := rows.Scan(&inst.ObjectID, &inst.TemplateID, &inst.Count, &inst.EnchantLevel,
			&loc, &inst.LocationData, &inst.CustomType1, &inst.CustomType2, &inst.ManaLeft, &inst.Time); err != nil {
			return nil, fmt.Errorf("list items for owner %d: %w", ownerID, err)
		}
		inst.Location, err = item.ParseLocation(loc)
		if err != nil {
			return nil, fmt.Errorf("list items for owner %d: %w", ownerID, err)
		}
		out = append(out, &inst)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list items for owner %d: %w", ownerID, err)
	}
	return out, nil
}

// DeleteByOwner removes every items row owned by ownerID and reports how
// many rows were deleted.
func (s *ItemStore) DeleteByOwner(ownerID int32) (int64, error) {
	res, err := s.db.Exec("DELETE FROM items WHERE owner_id = ?", ownerID)
	if err != nil {
		return 0, fmt.Errorf("delete items for owner %d: %w", ownerID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("delete items for owner %d: %w", ownerID, err)
	}
	return n, nil
}
