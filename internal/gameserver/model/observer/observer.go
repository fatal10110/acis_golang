// Package observer models observer-group XML data loaded at boot.
package observer

import (
	"fmt"
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

// Spawn is one observer NPC spawn entry with its allowed group ids.
type Spawn struct {
	NPCID    int
	Location location.Location
	Groups   []int
}

// ParseGroups parses a spawn's ";"-separated group id list.
func ParseGroups(raw string) ([]int, error) {
	parts := strings.Split(raw, ";")
	groups := make([]int, 0, len(parts))
	for _, part := range parts {
		groupID, err := commons.Atoi(part)
		if err != nil {
			return nil, fmt.Errorf("groups %q: %w", raw, err)
		}
		groups = append(groups, groupID)
	}
	return groups, nil
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
