package clan

import (
	"cmp"
	"slices"
)

// RefreshLadder ranks the clans again (see rankLadder). No member is told;
// the new rank shows in the next clan window.
func (t *Table) RefreshLadder() {
	rankLadder(t.allClans())
}

// rankLadder takes every clan's rank away, then walks the 99 best stored
// scores, best first and equal scores by clan id, giving ranks 1, 2, ...
// to the clans among them that hold a positive score. A clan restored
// below level 5 with a positive stored score holds none, so it takes one
// of the 99 slots without a rank.
func rankLadder(clans []*Clan) {
	type standing struct {
		cl     *Clan
		stored int
	}
	ladder := make([]standing, 0, len(clans))
	for _, cl := range clans {
		cl.mu.Lock()
		cl.rank = 0
		ladder = append(ladder, standing{cl, cl.storedReputation})
		cl.mu.Unlock()
	}
	slices.SortFunc(ladder, func(a, b standing) int {
		if c := cmp.Compare(b.stored, a.stored); c != 0 {
			return c
		}
		return cmp.Compare(a.cl.id, b.cl.id)
	})
	rank := 1
	for _, s := range ladder[:min(len(ladder), ladderSize)] {
		s.cl.mu.Lock()
		if s.cl.reputation > 0 {
			s.cl.rank = rank
			rank++
		}
		s.cl.mu.Unlock()
	}
}
