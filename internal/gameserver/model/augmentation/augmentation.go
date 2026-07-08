// Package augmentation models static life-stone augmentation data loaded at boot.
package augmentation

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/commons"
)

const (
	skillStart     = 14561
	skillBlockSize = 178
	skillLevels    = 10
)

// Color is an augmentation skill color bucket.
type Color string

// Augmentation skill color buckets.
const (
	Blue   Color = "blue"
	Purple Color = "purple"
	Red    Color = "red"
)

// Skill is one augmentation skill option.
type Skill struct {
	ID         int
	SkillID    int32
	SkillLevel int
	Color      Color
	Level      int
}

// NewSkill builds a Skill from one folded <augmentation> element.
func NewSkill(set *commons.StatSet) (Skill, error) {
	id, err := set.GetInt("id")
	if err != nil {
		return Skill{}, fmt.Errorf("augmentation skill: %w", err)
	}
	wrap := func(err error) error { return fmt.Errorf("augmentation skill %d: %w", id, err) }
	skillID, err := set.GetInt32("skillId")
	if err != nil {
		return Skill{}, wrap(err)
	}
	skillLevel, err := set.GetInt("skillLevel")
	if err != nil {
		return Skill{}, wrap(err)
	}
	rawColor, err := set.GetString("type")
	if err != nil {
		return Skill{}, wrap(err)
	}
	color := Color(rawColor)
	if color != Blue && color != Purple && color != Red {
		return Skill{}, wrap(fmt.Errorf("unknown color %q", rawColor))
	}
	level := (id - skillStart) / skillBlockSize
	if level < 0 || level >= skillLevels {
		return Skill{}, wrap(fmt.Errorf("id outside skill blocks"))
	}
	return Skill{ID: id, SkillID: skillID, SkillLevel: skillLevel, Color: color, Level: level}, nil
}

// Stat is one augmentation stat table.
type Stat struct {
	Name           string
	SoloValues     []float32
	CombinedValues []float32
}

// NewStat builds a Stat from its table values.
func NewStat(name string, solo, combined []float32) (Stat, error) {
	if name == "" {
		return Stat{}, fmt.Errorf("augmentation stat: name required")
	}
	if len(solo) == 0 {
		return Stat{}, fmt.Errorf("augmentation stat %s: solo values required", name)
	}
	if len(combined) == 0 {
		return Stat{}, fmt.Errorf("augmentation stat %s: combined values required", name)
	}
	return Stat{Name: name, SoloValues: solo, CombinedValues: combined}, nil
}

// StatGroup is one ordered color/stat block.
type StatGroup struct {
	Order int
	Stats []Stat
}

// NewStatGroup builds a StatGroup from one <set> element.
func NewStatGroup(set *commons.StatSet, stats []Stat) (StatGroup, error) {
	order, err := set.GetInt("order")
	if err != nil {
		return StatGroup{}, fmt.Errorf("augmentation stat group: %w", err)
	}
	return StatGroup{Order: order, Stats: stats}, nil
}

// Table stores augmentation stat groups and skill options.
type Table struct {
	StatGroups []StatGroup
	Skills     []Skill
	bySkillID  map[int]Skill
	Blue       [skillLevels][]int
	Purple     [skillLevels][]int
	Red        [skillLevels][]int
}

// NewTable builds an augmentation lookup table.
func NewTable(groups []StatGroup, skills []Skill) (*Table, error) {
	t := &Table{
		StatGroups: append([]StatGroup(nil), groups...),
		Skills:     append([]Skill(nil), skills...),
		bySkillID:  make(map[int]Skill, len(skills)),
	}
	for _, s := range skills {
		t.bySkillID[s.ID] = s
		switch s.Color {
		case Blue:
			t.Blue[s.Level] = append(t.Blue[s.Level], s.ID)
		case Purple:
			t.Purple[s.Level] = append(t.Purple[s.Level], s.ID)
		case Red:
			t.Red[s.Level] = append(t.Red[s.Level], s.ID)
		default:
			return nil, fmt.Errorf("augmentation skill %d: unknown color %q", s.ID, s.Color)
		}
	}
	return t, nil
}

// FindSkill returns the augmentation skill option with id.
func (t *Table) FindSkill(id int) (Skill, bool) {
	s, ok := t.bySkillID[id]
	return s, ok
}

// SkillCount returns the number of augmentation skill options.
func (t *Table) SkillCount() int {
	return len(t.Skills)
}

// StatCount returns the number of augmentation stat tables.
func (t *Table) StatCount() int {
	n := 0
	for _, g := range t.StatGroups {
		n += len(g.Stats)
	}
	return n
}
