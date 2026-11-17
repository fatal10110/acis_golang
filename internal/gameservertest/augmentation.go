package gameservertest

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// SeedAugmentation stores aug as the augmentation of the item itemObjectID
// names, as a saved character's augmented item would hold it.
func SeedAugmentation(ctx context.Context, db *sql.DB, itemObjectID int32, aug item.Augmentation) error {
	_, err := db.ExecContext(ctx,
		`INSERT INTO augmentations (item_oid, attributes, skill_id, skill_level) VALUES (?,?,?,?)`,
		itemObjectID, aug.Attributes, aug.SkillID, aug.SkillLevel,
	)
	if err != nil {
		return fmt.Errorf("seed augmentation for item %d: %w", itemObjectID, err)
	}
	return nil
}

// StoredAugmentation returns the saved augmentation of the item
// itemObjectID names, and false when it carries none.
func StoredAugmentation(ctx context.Context, db *sql.DB, itemObjectID int32) (item.Augmentation, bool, error) {
	var aug item.Augmentation
	err := db.QueryRowContext(ctx,
		`SELECT attributes, skill_id, skill_level FROM augmentations WHERE item_oid = ?`, itemObjectID,
	).Scan(&aug.Attributes, &aug.SkillID, &aug.SkillLevel)
	if errors.Is(err, sql.ErrNoRows) {
		return item.Augmentation{}, false, nil
	}
	if err != nil {
		return item.Augmentation{}, false, fmt.Errorf("read augmentation for item %d: %w", itemObjectID, err)
	}
	return aug, true, nil
}
