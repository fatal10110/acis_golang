package sql

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// ItemFlushStore persists a task.ItemInstances flush batch as one
// transaction spanning the items, augmentations, and pets tables, so a
// flush interrupted partway through leaves none of its changes visible
// rather than some of them.
type ItemFlushStore struct {
	db *sql.DB
}

// NewItemFlushStore returns an ItemFlushStore backed by db.
func NewItemFlushStore(db *sql.DB) *ItemFlushStore {
	return &ItemFlushStore{db: db}
}

// Flush writes every group in batch inside a single transaction: either all
// of it lands or, on any error, none of it does.
func (s *ItemFlushStore) Flush(ctx context.Context, batch item.FlushBatch) error {
	if len(batch.Saves) == 0 && len(batch.Deletes) == 0 && len(batch.AugmentationSaves) == 0 &&
		len(batch.AugmentationDeletes) == 0 && len(batch.PetDeletes) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin item flush: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	if err := flushItemSaves(ctx, tx, batch.Saves); err != nil {
		return err
	}
	if err := flushInt32Delete(ctx, tx, "DELETE FROM items WHERE object_id IN (%s)", batch.Deletes); err != nil {
		return fmt.Errorf("delete items: %w", err)
	}
	if err := flushAugmentationSaves(ctx, tx, batch.AugmentationSaves); err != nil {
		return err
	}
	if err := flushInt32Delete(ctx, tx, "DELETE FROM augmentations WHERE item_oid IN (%s)", batch.AugmentationDeletes); err != nil {
		return fmt.Errorf("delete augmentations: %w", err)
	}
	if err := flushInt32Delete(ctx, tx, "DELETE FROM pets WHERE item_obj_id IN (%s)", batch.PetDeletes); err != nil {
		return fmt.Errorf("delete pet items: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit item flush: %w", err)
	}
	committed = true
	return nil
}

func flushItemSaves(ctx context.Context, tx *sql.Tx, saves []*item.Instance) error {
	if len(saves) == 0 {
		return nil
	}

	placeholders := make([]string, len(saves))
	args := make([]any, 0, len(saves)*11)
	for i, inst := range saves {
		st := inst.Snapshot()
		placeholders[i] = "(?,?,?,?,?,?,?,?,?,?,?)"
		args = append(args, st.OwnerID, st.ObjectID, st.TemplateID, st.Count, st.EnchantLevel,
			st.Location.String(), st.LocationData, st.CustomType1, st.CustomType2, st.ManaLeft, st.Time)
	}

	query := fmt.Sprintf(`INSERT INTO items
			(owner_id, object_id, item_id, count, enchant_level, loc, loc_data, custom_type1, custom_type2, mana_left, time)
		 VALUES %s
		 ON DUPLICATE KEY UPDATE
			owner_id=VALUES(owner_id), count=VALUES(count), loc=VALUES(loc), loc_data=VALUES(loc_data),
			enchant_level=VALUES(enchant_level), custom_type1=VALUES(custom_type1), custom_type2=VALUES(custom_type2),
			mana_left=VALUES(mana_left), time=VALUES(time)`, strings.Join(placeholders, ","))

	if _, err := tx.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("save items: %w", err)
	}
	return nil
}

func flushAugmentationSaves(ctx context.Context, tx *sql.Tx, saves []item.FlushAugmentationSave) error {
	if len(saves) == 0 {
		return nil
	}

	placeholders := make([]string, len(saves))
	args := make([]any, 0, len(saves)*4)
	for i, save := range saves {
		placeholders[i] = "(?,?,?,?)"
		args = append(args, save.ObjectID, save.Augmentation.Attributes, save.Augmentation.SkillID, save.Augmentation.SkillLevel)
	}

	query := fmt.Sprintf(`INSERT INTO augmentations (item_oid, attributes, skill_id, skill_level) VALUES %s
		 ON DUPLICATE KEY UPDATE attributes=VALUES(attributes), skill_id=VALUES(skill_id), skill_level=VALUES(skill_level)`,
		strings.Join(placeholders, ","))

	if _, err := tx.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("save augmentations: %w", err)
	}
	return nil
}

func flushInt32Delete(ctx context.Context, tx *sql.Tx, queryFmt string, ids []int32) error {
	if len(ids) == 0 {
		return nil
	}

	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}

	if _, err := tx.ExecContext(ctx, fmt.Sprintf(queryFmt, placeholders), args...); err != nil {
		return err
	}
	return nil
}
