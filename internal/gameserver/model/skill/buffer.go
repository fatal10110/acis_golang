package skill

import (
	"fmt"
	"strings"

	"github.com/fatal10110/acis_golang/internal/commons"
)

// BufferSkill is one scheme-buffer skill entry loaded from bufferSkills.xml.
type BufferSkill struct {
	Skill       Ref
	Price       int
	Category    string
	Description string
}

// NewBufferSkill builds a BufferSkill from one folded XML buff element.
func NewBufferSkill(set *commons.StatSet, skills *Table) (BufferSkill, error) {
	idf := commons.NewFields(set, "skill: buffer skill")
	skillID := idf.Int32("id")
	if err := idf.Err(); err != nil {
		return BufferSkill{}, err
	}

	f := commons.NewFields(set, fmt.Sprintf("skill: buffer skill %d", skillID))
	category := f.String("type")
	if err := f.Err(); err != nil {
		return BufferSkill{}, err
	}
	level := 0
	if f.Has("level") {
		level = f.Int("level")
		if err := f.Err(); err != nil {
			return BufferSkill{}, err
		}
	} else {
		if skills == nil {
			return BufferSkill{}, fmt.Errorf("skill: buffer skill %d: missing skill table", skillID)
		}
		level = skills.MaxLevel(ID(skillID))
		if level <= 0 {
			return BufferSkill{}, fmt.Errorf("skill: buffer skill %d: skill not found", skillID)
		}
	}

	entry := BufferSkill{
		Skill:       Ref{ID: ID(skillID), Level: level},
		Price:       f.IntDefault("price", 0),
		Category:    category,
		Description: f.StringDefault("desc", ""),
	}
	if err := f.Err(); err != nil {
		return BufferSkill{}, err
	}
	return entry, nil
}

// BufferTable is an in-memory lookup of scheme-buffer skills by id.
type BufferTable struct {
	byID       map[ID]BufferSkill
	categories []string
}

// NewBufferTable builds a BufferTable and preserves first-seen category order.
func NewBufferTable(entries []BufferSkill) (*BufferTable, error) {
	byID := make(map[ID]BufferSkill, len(entries))
	categories := make([]string, 0, len(entries))
	seenCategories := make(map[string]struct{}, len(entries))

	for _, entry := range entries {
		if _, exists := byID[entry.Skill.ID]; exists {
			return nil, fmt.Errorf("skill: duplicate buffer skill %d", entry.Skill.ID)
		}
		byID[entry.Skill.ID] = entry

		key := strings.ToLower(entry.Category)
		if _, exists := seenCategories[key]; exists {
			continue
		}
		seenCategories[key] = struct{}{}
		categories = append(categories, entry.Category)
	}

	return &BufferTable{byID: byID, categories: categories}, nil
}

// Count returns the number of scheme-buffer skills in the table.
func (t *BufferTable) Count() int {
	return len(t.byID)
}

// Skill returns the scheme-buffer entry for skillID, if present.
func (t *BufferTable) Skill(skillID int32) (BufferSkill, bool) {
	entry, ok := t.byID[ID(skillID)]
	return entry, ok
}

// Categories returns the distinct skill categories in first-seen order.
func (t *BufferTable) Categories() []string {
	return append([]string(nil), t.categories...)
}
