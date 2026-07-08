// Package armorset models static armor set data loaded at boot.
package armorset

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/commons"
)

// Set is one armor set template.
type Set struct {
	Name          string
	Chest         int32
	Legs          int32
	Head          int32
	Gloves        int32
	Feet          int32
	SkillID       int32
	Shield        int32
	ShieldSkillID int32
	Enchant6Skill int32
}

// New builds a Set from one folded <armorset> element.
func New(attrs *commons.StatSet) (Set, error) {
	name, err := attrs.GetString("name")
	if err != nil {
		return Set{}, fmt.Errorf("armorset: %w", err)
	}
	wrap := func(err error) error { return fmt.Errorf("armorset %q: %w", name, err) }

	chest, err := attrs.GetInt32("chest")
	if err != nil {
		return Set{}, wrap(err)
	}
	legs, err := attrs.GetInt32("legs")
	if err != nil {
		return Set{}, wrap(err)
	}
	head, err := attrs.GetInt32("head")
	if err != nil {
		return Set{}, wrap(err)
	}
	gloves, err := attrs.GetInt32("gloves")
	if err != nil {
		return Set{}, wrap(err)
	}
	feet, err := attrs.GetInt32("feet")
	if err != nil {
		return Set{}, wrap(err)
	}
	skillID, err := attrs.GetInt32("skillId")
	if err != nil {
		return Set{}, wrap(err)
	}
	shield, err := attrs.GetInt32("shield")
	if err != nil {
		return Set{}, wrap(err)
	}
	shieldSkillID, err := attrs.GetInt32("shieldSkillId")
	if err != nil {
		return Set{}, wrap(err)
	}
	enchant6Skill, err := attrs.GetInt32("enchant6Skill")
	if err != nil {
		return Set{}, wrap(err)
	}
	return Set{
		Name: name, Chest: chest, Legs: legs, Head: head, Gloves: gloves, Feet: feet,
		SkillID: skillID, Shield: shield, ShieldSkillID: shieldSkillID, Enchant6Skill: enchant6Skill,
	}, nil
}

// PieceIDs returns chest, legs, head, gloves and feet item ids in paperdoll order.
func (s Set) PieceIDs() [5]int32 {
	return [5]int32{s.Chest, s.Legs, s.Head, s.Gloves, s.Feet}
}

// Table stores armor sets keyed by chest item id.
type Table struct {
	byChest map[int32]Set
}

// NewTable builds an armor set lookup table.
func NewTable(sets []Set) *Table {
	t := &Table{byChest: make(map[int32]Set, len(sets))}
	for _, s := range sets {
		t.byChest[s.Chest] = s
	}
	return t
}

// Len returns the number of armor sets keyed by chest item id.
func (t *Table) Len() int {
	return len(t.byChest)
}

// FindByChest returns the armor set whose chest piece is chestID.
func (t *Table) FindByChest(chestID int32) (Set, bool) {
	s, ok := t.byChest[chestID]
	return s, ok
}
