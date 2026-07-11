package sql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// AugmentationStore reads and writes the augmentations table: the
// life-stone bonus applied to individual item instances, keyed by the
// item's own object id.
type AugmentationStore struct {
	db *sql.DB
}

// NewAugmentationStore returns an AugmentationStore backed by db.
func NewAugmentationStore(db *sql.DB) *AugmentationStore {
	return &AugmentationStore{db: db}
}

// Create inserts aug as the augmentation applied to the item identified by
// itemObjectID.
func (s *AugmentationStore) Create(ctx context.Context, itemObjectID int32, aug item.Augmentation) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO augmentations (item_oid, attributes, skill_id, skill_level) VALUES (?,?,?,?)`,
		itemObjectID, aug.Attributes, aug.SkillID, aug.SkillLevel,
	)
	if err != nil {
		return fmt.Errorf("create augmentation for item %d: %w", itemObjectID, err)
	}
	return nil
}

// Get returns the augmentation applied to the item identified by
// itemObjectID, or (Augmentation{}, false) if it carries none.
func (s *AugmentationStore) Get(ctx context.Context, itemObjectID int32) (item.Augmentation, bool, error) {
	var aug item.Augmentation
	err := s.db.QueryRowContext(ctx,
		`SELECT attributes, skill_id, skill_level FROM augmentations WHERE item_oid = ?`, itemObjectID,
	).Scan(&aug.Attributes, &aug.SkillID, &aug.SkillLevel)
	if errors.Is(err, sql.ErrNoRows) {
		return item.Augmentation{}, false, nil
	}
	if err != nil {
		return item.Augmentation{}, false, fmt.Errorf("get augmentation for item %d: %w", itemObjectID, err)
	}
	return aug, true, nil
}

// Delete removes the augmentation applied to the item identified by
// itemObjectID, if any.
func (s *AugmentationStore) Delete(ctx context.Context, itemObjectID int32) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM augmentations WHERE item_oid = ?", itemObjectID)
	if err != nil {
		return fmt.Errorf("delete augmentation for item %d: %w", itemObjectID, err)
	}
	return nil
}
