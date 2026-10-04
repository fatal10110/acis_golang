package observer

import (
	"slices"
	"strconv"
	"strings"
)

// Location returns the viewpoint with locID: the first one found going
// through the groups by ascending group id, each in XML order. A locID
// several groups share resolves to the lowest group's entry, whichever
// tower's list offered it.
func (t *Table) Location(locID int) (Location, bool) {
	if t == nil {
		return Location{}, false
	}
	ids := make([]int, 0, len(t.groups))
	for id := range t.groups {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	for _, id := range ids {
		for _, loc := range t.groups[id] {
			if loc.ID == locID {
				return loc, true
			}
		}
	}
	return Location{}, false
}

// GroupsWindow is a broadcasting tower's first page: one link per group
// the tower offers, each opening that group's viewpoints. The client
// names a group and a viewpoint by its own string table, &$<id>;.
func GroupsWindow(objectID int32, groups []int) string {
	var b strings.Builder
	b.WriteString("<html><body>&$650;<br><br>")
	oid := strconv.Itoa(int(objectID))
	for _, id := range groups {
		g := strconv.Itoa(id)
		b.WriteString(`<a action="bypass -h npc_` + oid + `_observe_group ` + g + `">&$` + g + `;</a><br1>`)
	}
	b.WriteString("</body></html>")
	return b.String()
}

// LocationsWindow is a group's page: one link per viewpoint, followed by
// its fee in adena when it has one.
func LocationsWindow(objectID int32, locs []Location) string {
	var b strings.Builder
	b.WriteString("<html><body>&$650;<br><br>")
	oid := strconv.Itoa(int(objectID))
	for _, loc := range locs {
		id := strconv.Itoa(loc.ID)
		b.WriteString(`<a action="bypass -h npc_` + oid + `_observe ` + id + `">&$` + id + `;`)
		if loc.Cost > 0 {
			b.WriteString(" - " + strconv.Itoa(loc.Cost) + " &#57;")
		}
		b.WriteString("</a><br1>")
	}
	b.WriteString("</body></html>")
	return b.String()
}
