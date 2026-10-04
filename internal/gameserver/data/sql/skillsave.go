package sql

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

// SkillSaveStore reads and writes the character_skills_save table: each
// character's active buff/debuff state and pending skill reuse delays,
// persisted across logout so a relog can restore them.
type SkillSaveStore struct {
	db *sql.DB
}

// NewSkillSaveStore returns a SkillSaveStore backed by db.
func NewSkillSaveStore(db *sql.DB) *SkillSaveStore {
	return &SkillSaveStore{db: db}
}

// Replace deletes every row charObjID has stored for classIndex and inserts
// rows in their place — the delete-then-insert a logout performs each time,
// so a save that no longer carries a given skill's reuse timer doesn't
// leave a stale row behind from an earlier logout. Both run in one
// transaction: a failed insert, a lost connection or a crash leaves the
// previous rows whole, and no reader sees the character with none.
func (s *SkillSaveStore) Replace(ctx context.Context, charObjID int32, classIndex int32, rows []effect.SaveRow) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("save skill rows for character %d class %d: begin: %w", charObjID, classIndex, err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM character_skills_save WHERE char_obj_id = ? AND class_index = ?`,
		charObjID, classIndex,
	); err != nil {
		return fmt.Errorf("clear skill save rows for character %d class %d: %w", charObjID, classIndex, err)
	}

	if len(rows) > 0 {
		const columns = 10
		query := strings.Builder{}
		query.WriteString(`INSERT INTO character_skills_save
			(char_obj_id, skill_id, skill_level, effect_count, effect_cur_time, reuse_delay, systime, restore_type, class_index, buff_index)
			VALUES `)
		args := make([]any, 0, len(rows)*columns)
		for i, row := range rows {
			if i > 0 {
				query.WriteString(",")
			}
			query.WriteString("(?,?,?,?,?,?,?,?,?,?)")
			args = append(args, charObjID, row.Skill.ID, row.Skill.Level, row.EffectCount, row.EffectCurTime,
				row.ReuseDelay, row.SystemTime, row.RestoreType, classIndex, row.BuffIndex)
		}
		if _, err := tx.ExecContext(ctx, query.String(), args...); err != nil {
			return fmt.Errorf("save skill rows for character %d class %d: %w", charObjID, classIndex, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("save skill rows for character %d class %d: commit: %w", charObjID, classIndex, err)
	}
	return nil
}

// ListByCharacter returns charObjID's classIndex rows ordered by
// buff_index, the order they were saved in and the order a restore replays
// them in.
func (s *SkillSaveStore) ListByCharacter(ctx context.Context, charObjID int32, classIndex int32) ([]effect.SaveRow, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT skill_id, skill_level, effect_count, effect_cur_time, reuse_delay, systime, restore_type, buff_index
		 FROM character_skills_save WHERE char_obj_id = ? AND class_index = ? ORDER BY buff_index ASC`,
		charObjID, classIndex,
	)
	if err != nil {
		return nil, fmt.Errorf("list skill save rows for character %d class %d: %w", charObjID, classIndex, err)
	}
	defer rows.Close()

	out := []effect.SaveRow{}
	for rows.Next() {
		var row effect.SaveRow
		if err := rows.Scan(&row.Skill.ID, &row.Skill.Level, &row.EffectCount, &row.EffectCurTime,
			&row.ReuseDelay, &row.SystemTime, &row.RestoreType, &row.BuffIndex); err != nil {
			return nil, fmt.Errorf("list skill save rows for character %d class %d: %w", charObjID, classIndex, err)
		}
		row.ClassIndex = classIndex
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list skill save rows for character %d class %d: %w", charObjID, classIndex, err)
	}
	return out, nil
}

// DeleteByCharacter removes every row charObjID has stored for classIndex
// and reports how many rows were deleted — the cleanup a restore performs
// once it has consumed them.
func (s *SkillSaveStore) DeleteByCharacter(ctx context.Context, charObjID int32, classIndex int32) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM character_skills_save WHERE char_obj_id = ? AND class_index = ?`,
		charObjID, classIndex,
	)
	if err != nil {
		return 0, fmt.Errorf("delete skill save rows for character %d class %d: %w", charObjID, classIndex, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("delete skill save rows for character %d class %d: %w", charObjID, classIndex, err)
	}
	return n, nil
}
