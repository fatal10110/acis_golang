// Package fish models static fishing creature data loaded at boot.
package fish

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/commons"
)

// Fish is one static fish row.
type Fish struct {
	ID            int32
	Level         int
	HP            int
	HPRegen       int
	Type          int
	Group         int
	Guts          int
	GutsCheckTime int
	WaitTime      int
	CombatTime    int
}

// New builds a Fish from one folded <fish> element.
func New(set *commons.StatSet) (Fish, error) {
	idf := commons.NewFields(set, "fish")
	id := idf.Int32("id")
	if err := idf.Err(); err != nil {
		return Fish{}, err
	}

	f := commons.NewFields(set, fmt.Sprintf("fish %d", id))
	fish := Fish{
		ID:            id,
		Level:         f.Int("level"),
		HP:            f.Int("hp"),
		HPRegen:       f.Int("hpRegen"),
		Type:          f.Int("type"),
		Group:         f.Int("group"),
		Guts:          f.Int("guts"),
		GutsCheckTime: f.Int("gutsCheckTime"),
		WaitTime:      f.Int("waitTime"),
		CombatTime:    f.Int("combatTime"),
	}
	if err := f.Err(); err != nil {
		return Fish{}, err
	}
	return fish, nil
}

// Table stores fish rows.
type Table struct {
	fish []Fish
	byID map[int32]Fish
}

// NewTable builds a fish lookup table.
func NewTable(rows []Fish) *Table {
	t := &Table{fish: append([]Fish(nil), rows...), byID: make(map[int32]Fish, len(rows))}
	for _, f := range rows {
		t.byID[f.ID] = f
	}
	return t
}

// Len returns the number of fish rows.
func (t *Table) Len() int {
	return len(t.fish)
}

// Find returns the fish with id.
func (t *Table) Find(id int32) (Fish, bool) {
	f, ok := t.byID[id]
	return f, ok
}
