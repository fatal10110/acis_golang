package skill

import (
	"fmt"
)

// BufferSkill is one scheme-buffer skill entry loaded from bufferSkills.xml.
type BufferSkill struct {
	Skill       Ref
	Price       int
	Category    string
	Description string
}

// NewBufferSkill builds a BufferSkill from one <buff> element's decoded
// attributes and its parent <category>'s type. level is nil when the
// element omits it, in which case it is looked up as the skill's max level
// in skills.
func NewBufferSkill(skillID int32, category string, level *int, price int, description string, skills *Table) (BufferSkill, error) {
	lvl := 0
	if level != nil {
		lvl = *level
	} else {
		if skills == nil {
			return BufferSkill{}, fmt.Errorf("skill: buffer skill %d: missing skill table", skillID)
		}
		lvl = skills.MaxLevel(ID(skillID))
	}

	return BufferSkill{
		Skill:       Ref{ID: ID(skillID), Level: lvl},
		Price:       price,
		Category:    category,
		Description: description,
	}, nil
}

// BufferTable is an in-memory lookup of scheme-buffer skills by id.
type BufferTable struct {
	byID       map[ID]BufferSkill
	categories []string
}

// NewBufferTable builds a BufferTable and preserves category order after duplicate replacement.
func NewBufferTable(entries []BufferSkill) (*BufferTable, error) {
	byID := make(map[ID]BufferSkill, len(entries))
	ids := make([]ID, 0, len(entries))
	for _, entry := range entries {
		if _, exists := byID[entry.Skill.ID]; !exists {
			ids = append(ids, entry.Skill.ID)
		}
		byID[entry.Skill.ID] = entry
	}

	categories := make([]string, 0, len(entries))
	seenCategories := make(map[string]struct{}, len(entries))
	for _, id := range ids {
		entry := byID[id]
		if _, exists := seenCategories[entry.Category]; exists {
			continue
		}
		seenCategories[entry.Category] = struct{}{}
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
