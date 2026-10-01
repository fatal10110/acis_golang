package clan

import (
	"sort"
	"strconv"
)

// WarListKind selects one of the war listings a clan member can ask for.
type WarListKind uint8

// War listings.
const (
	// WarsDeclared lists the clans the clan declared war on that did not
	// declare war back.
	WarsDeclared WarListKind = iota
	// WarsAgainst lists the clans that declared war on the clan that it
	// did not declare war back on.
	WarsAgainst
	// WarsMutual lists the clans at war with the clan both ways.
	WarsMutual
)

// WarListing returns the headers of the clans on cl's stored war rows of
// the given kind.
//
// The listing reads the stored rows, not the wars in force: a stopped war
// whose re-declaration penalty is still stored keeps its row, and so its
// place in the listing, until the row is dropped at the next boot or a new
// declaration replaces it. The rows store clan ids as text and are read in
// that key order, so the entries sort by the other clan's id compared as a
// decimal string.
func (s *Service) WarListing(cl *Clan, kind WarListKind) []Info {
	own := cl.warRows()
	// against holds the clans with a row on cl.
	against := map[int32]bool{}
	for _, other := range s.table.allClans() {
		if other != cl && other.warRows()[cl.id] {
			against[other.id] = true
		}
	}
	var ids []int32
	switch kind {
	case WarsDeclared:
		for id := range own {
			if !against[id] {
				ids = append(ids, id)
			}
		}
	case WarsAgainst:
		for id := range against {
			if !own[id] {
				ids = append(ids, id)
			}
		}
	case WarsMutual:
		for id := range own {
			if against[id] {
				ids = append(ids, id)
			}
		}
	}
	sort.Slice(ids, func(i, j int) bool {
		return strconv.Itoa(int(ids[i])) < strconv.Itoa(int(ids[j]))
	})
	out := make([]Info, 0, len(ids))
	for _, id := range ids {
		if other, ok := s.table.Get(id); ok {
			out = append(out, other.Info())
		}
	}
	return out
}

// warRows returns the clans cl has a stored war row on: every war it
// declared, and every clan it keeps a re-declaration penalty on.
func (cl *Clan) warRows() map[int32]bool {
	cl.mu.RLock()
	defer cl.mu.RUnlock()
	rows := make(map[int32]bool, len(cl.wars)+len(cl.warPenalties))
	for id := range cl.wars {
		rows[id] = true
	}
	for id := range cl.warPenalties {
		rows[id] = true
	}
	return rows
}
