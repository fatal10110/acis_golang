// Package observer models observer-group XML data loaded at boot.
package observer

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// Location is one observer viewpoint entry.
type Location struct {
	ID       int
	Location location.Location
	Yaw      int
	Pitch    int
	Cost     int
	CastleID int
}

// NewLocation builds one observer location from XML attributes.
func NewLocation(set *commons.StatSet) (Location, error) {
	idf := commons.NewFields(set, "observer location")
	id := idf.Int("locId")
	if err := idf.Err(); err != nil {
		return Location{}, err
	}
	wrap := func(err error) error { return fmt.Errorf("observer location %d: %w", id, err) }

	loc, err := location.NewLocation(set)
	if err != nil {
		return Location{}, wrap(err)
	}

	f := commons.NewFields(set, fmt.Sprintf("observer location %d", id))
	location := Location{
		ID:       id,
		Location: loc,
		Yaw:      f.Int("yaw"),
		Pitch:    f.Int("pitch"),
		Cost:     f.Int("cost"),
		CastleID: f.Int("castle"),
	}
	if err := f.Err(); err != nil {
		return Location{}, err
	}
	return location, nil
}

// Spawn is one observer NPC spawn entry with its allowed group ids.
type Spawn struct {
	NPCID    int
	Location location.Location
	Groups   []int
}

// NewSpawn builds one observer spawn from XML attributes.
func NewSpawn(set *commons.StatSet) (Spawn, error) {
	npcID, err := set.GetInt("id")
	if err != nil {
		return Spawn{}, fmt.Errorf("observer spawn: %w", err)
	}
	wrap := func(err error) error { return fmt.Errorf("observer spawn %d: %w", npcID, err) }

	loc, err := location.NewLocation(set)
	if err != nil {
		return Spawn{}, wrap(err)
	}
	groupText, err := set.GetString("groups")
	if err != nil {
		return Spawn{}, wrap(err)
	}
	parts := strings.Split(groupText, ";")
	groups := make([]int, 0, len(parts))
	for _, part := range parts {
		groupID, err := strconv.Atoi(part)
		if err != nil {
			return Spawn{}, wrap(fmt.Errorf("groups %q: %w", groupText, err))
		}
		groups = append(groups, groupID)
	}
	return Spawn{NPCID: npcID, Location: loc, Groups: groups}, nil
}

// Table stores observer groups keyed by group id plus observer spawns.
type Table struct {
	groups map[int][]Location
	spawns []Spawn
}

// NewTable builds an observer-group table.
func NewTable(groups map[int][]Location, spawns []Spawn) *Table {
	groupMap := make(map[int][]Location, len(groups))
	for id, entries := range groups {
		groupMap[id] = append([]Location(nil), entries...)
	}
	return &Table{
		groups: groupMap,
		spawns: append([]Spawn(nil), spawns...),
	}
}

// GroupCount returns the number of observer groups.
func (t *Table) GroupCount() int {
	if t == nil {
		return 0
	}
	return len(t.groups)
}

// SpawnCount returns the number of observer spawns.
func (t *Table) SpawnCount() int {
	if t == nil {
		return 0
	}
	return len(t.spawns)
}

// Group returns the group's locations in XML order.
func (t *Table) Group(id int) ([]Location, bool) {
	if t == nil {
		return nil, false
	}
	entries, ok := t.groups[id]
	if !ok {
		return nil, false
	}
	return append([]Location(nil), entries...), true
}

// Spawns returns the observer spawns in XML order.
func (t *Table) Spawns() []Spawn {
	if t == nil {
		return nil
	}
	return append([]Spawn(nil), t.spawns...)
}
