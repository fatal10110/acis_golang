package sql

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
)

// CharacterSkillStore reads learned skill levels from character_skills.
type CharacterSkillStore struct {
	db *sql.DB
}

// NewCharacterSkillStore returns a CharacterSkillStore backed by db.
func NewCharacterSkillStore(db *sql.DB) *CharacterSkillStore {
	return &CharacterSkillStore{db: db}
}

// ListKnownSkills returns the learned skill levels for one character class.
func (s *CharacterSkillStore) ListKnownSkills(ctx context.Context, charObjID int32, classIndex int32) (player.SkillLevels, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT skill_id, skill_level FROM character_skills WHERE char_obj_id = ? AND class_index = ?`,
		charObjID, classIndex,
	)
	if err != nil {
		return nil, fmt.Errorf("list known skills for character %d class %d: %w", charObjID, classIndex, err)
	}
	defer rows.Close()

	levels := player.SkillLevels{}
	for rows.Next() {
		var id, level int
		if err := rows.Scan(&id, &level); err != nil {
			return nil, fmt.Errorf("list known skills for character %d class %d: %w", charObjID, classIndex, err)
		}
		levels[id] = level
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list known skills for character %d class %d: %w", charObjID, classIndex, err)
	}
	return levels, nil
}

// SetKnownSkill persists one learned skill level for one character class.
func (s *CharacterSkillStore) SetKnownSkill(ctx context.Context, charObjID int32, classIndex int32, skillID int, level int) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO character_skills (char_obj_id, skill_id, skill_level, class_index)
		 VALUES (?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE skill_level = VALUES(skill_level)`,
		charObjID, skillID, level, classIndex,
	)
	if err != nil {
		return fmt.Errorf("set known skill %d level %d for character %d class %d: %w", skillID, level, charObjID, classIndex, err)
	}
	return nil
}
