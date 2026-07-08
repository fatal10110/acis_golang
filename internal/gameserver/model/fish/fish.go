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
	id, err := set.GetInt32("id")
	if err != nil {
		return Fish{}, fmt.Errorf("fish: %w", err)
	}
	wrap := func(err error) error { return fmt.Errorf("fish %d: %w", id, err) }
	level, err := set.GetInt("level")
	if err != nil {
		return Fish{}, wrap(err)
	}
	hp, err := set.GetInt("hp")
	if err != nil {
		return Fish{}, wrap(err)
	}
	hpRegen, err := set.GetInt("hpRegen")
	if err != nil {
		return Fish{}, wrap(err)
	}
	fishType, err := set.GetInt("type")
	if err != nil {
		return Fish{}, wrap(err)
	}
	group, err := set.GetInt("group")
	if err != nil {
		return Fish{}, wrap(err)
	}
	guts, err := set.GetInt("guts")
	if err != nil {
		return Fish{}, wrap(err)
	}
	gutsCheckTime, err := set.GetInt("gutsCheckTime")
	if err != nil {
		return Fish{}, wrap(err)
	}
	waitTime, err := set.GetInt("waitTime")
	if err != nil {
		return Fish{}, wrap(err)
	}
	combatTime, err := set.GetInt("combatTime")
	if err != nil {
		return Fish{}, wrap(err)
	}
	return Fish{
		ID: id, Level: level, HP: hp, HPRegen: hpRegen, Type: fishType, Group: group,
		Guts: guts, GutsCheckTime: gutsCheckTime, WaitTime: waitTime, CombatTime: combatTime,
	}, nil
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
