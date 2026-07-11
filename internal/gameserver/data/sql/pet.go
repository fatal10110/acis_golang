package sql

import (
	"context"
	"database/sql"
	"fmt"
)

// PetStore writes pet rows keyed by the pet collar item object id.
type PetStore struct {
	db *sql.DB
}

// NewPetStore returns a PetStore backed by db.
func NewPetStore(db *sql.DB) *PetStore {
	return &PetStore{db: db}
}

// DeleteByItemObjectID removes the pet row tied to itemObjectID, if any.
func (s *PetStore) DeleteByItemObjectID(ctx context.Context, itemObjectID int32) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM pets WHERE item_obj_id = ?", itemObjectID)
	if err != nil {
		return fmt.Errorf("delete pet item %d: %w", itemObjectID, err)
	}
	return nil
}
