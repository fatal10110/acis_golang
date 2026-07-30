package sql

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// itemFlushChunkSize bounds how many rows one multi-row statement covers.
// MySQL/MariaDB caps a prepared statement at 65535 placeholders; the items
// row (11 params) is the widest group here, so 1000 rows per statement
// stays far under that ceiling for every group while still keeping a
// large flush down to a handful of round trips.
const itemFlushChunkSize = 1000

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
	if err := flushInt32Delete(ctx, tx, "items", "object_id", batch.Deletes); err != nil {
		return err
	}
	if err := flushAugmentationSaves(ctx, tx, batch.AugmentationSaves); err != nil {
		return err
	}
	if err := flushInt32Delete(ctx, tx, "augmentations", "item_oid", batch.AugmentationDeletes); err != nil {
		return err
	}
	if err := flushInt32Delete(ctx, tx, "pets", "item_obj_id", batch.PetDeletes); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit item flush: %w", err)
	}
	committed = true
	return nil
}

// flushItemSaves reads saves' fields directly: FlushBatch.Saves holds
// point-in-time state, not a live instance, so nothing else can be
// mutating it concurrently.
func flushItemSaves(ctx context.Context, tx *sql.Tx, saves []item.InstanceState) error {
	for chunk := range slices.Chunk(saves, itemFlushChunkSize) {
		placeholders := make([]string, len(chunk))
		args := make([]any, 0, len(chunk)*11)
		for i, inst := range chunk {
			placeholders[i] = "(?,?,?,?,?,?,?,?,?,?,?)"
			args = append(args, inst.OwnerID, inst.ObjectID, inst.TemplateID, inst.Count, inst.EnchantLevel,
				inst.Location.String(), inst.LocationData, inst.CustomType1, inst.CustomType2, inst.ManaLeft, inst.Time)
		}

		query := fmt.Sprintf(`INSERT INTO items
				(owner_id, object_id, item_id, count, enchant_level, loc, loc_data, custom_type1, custom_type2, mana_left, time)
			 VALUES %s
			 ON DUPLICATE KEY UPDATE
				owner_id=VALUES(owner_id), count=VALUES(count), loc=VALUES(loc), loc_data=VALUES(loc_data),
				enchant_level=VALUES(enchant_level), custom_type1=VALUES(custom_type1), custom_type2=VALUES(custom_type2),
				mana_left=VALUES(mana_left), time=VALUES(time)`, strings.Join(placeholders, ","))

		if _, err := tx.ExecContext(ctx, query, args...); err != nil {
			return fmt.Errorf("save %d items (object ids %d..%d): %w", len(chunk), chunk[0].ObjectID, chunk[len(chunk)-1].ObjectID, err)
		}
	}
	return nil
}

func flushAugmentationSaves(ctx context.Context, tx *sql.Tx, saves []item.FlushAugmentationSave) error {
	for chunk := range slices.Chunk(saves, itemFlushChunkSize) {
		placeholders := make([]string, len(chunk))
		args := make([]any, 0, len(chunk)*4)
		for i, save := range chunk {
			placeholders[i] = "(?,?,?,?)"
			args = append(args, save.ObjectID, save.Augmentation.Attributes, save.Augmentation.SkillID, save.Augmentation.SkillLevel)
		}

		query := fmt.Sprintf(`INSERT INTO augmentations (item_oid, attributes, skill_id, skill_level) VALUES %s
			 ON DUPLICATE KEY UPDATE attributes=VALUES(attributes), skill_id=VALUES(skill_id), skill_level=VALUES(skill_level)`,
			strings.Join(placeholders, ","))

		if _, err := tx.ExecContext(ctx, query, args...); err != nil {
			return fmt.Errorf("save %d augmentations (object ids %d..%d): %w", len(chunk), chunk[0].ObjectID, chunk[len(chunk)-1].ObjectID, err)
		}
	}
	return nil
}

// flushInt32Delete deletes every row in table whose column matches one of
// ids, building the statement from table and column itself so the query
// and the error message it can produce always name the same table.
func flushInt32Delete(ctx context.Context, tx *sql.Tx, table, column string, ids []int32) error {
	for chunk := range slices.Chunk(ids, itemFlushChunkSize) {
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(chunk)), ",")
		args := make([]any, len(chunk))
		for i, id := range chunk {
			args[i] = id
		}

		query := fmt.Sprintf("DELETE FROM %s WHERE %s IN (%s)", table, column, placeholders)
		if _, err := tx.ExecContext(ctx, query, args...); err != nil {
			return fmt.Errorf("delete %d %s (object ids %d..%d): %w", len(chunk), table, chunk[0], chunk[len(chunk)-1], err)
		}
	}
	return nil
}
