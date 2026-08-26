// Package fish models static fishing creature data loaded at boot.
package fish

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

// New builds a Fish from decoded <fish> attributes.
func New(id int32, level, hp, hpRegen, typ, group, guts, gutsCheckTime, waitTime, combatTime int) Fish {
	return Fish{
		ID:            id,
		Level:         level,
		HP:            hp,
		HPRegen:       hpRegen,
		Type:          typ,
		Group:         group,
		Guts:          guts,
		GutsCheckTime: gutsCheckTime,
		WaitTime:      waitTime,
		CombatTime:    combatTime,
	}
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
