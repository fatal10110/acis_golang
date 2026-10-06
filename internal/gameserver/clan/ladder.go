package clan

import (
	"cmp"
	"slices"
)

// RefreshLadder ranks the clans again from their current reputation: every
// clan loses its rank, then among the 99 best scores, best first and equal
// scores by clan id, the clans with a positive score take ranks 1, 2, ...
// No member is told; the new rank shows in the next clan window.
func (t *Table) RefreshLadder() {
	type standing struct {
		cl         *Clan
		reputation int
	}
	clans := t.allClans()
	ladder := make([]standing, 0, len(clans))
	for _, cl := range clans {
		cl.mu.Lock()
		cl.rank = 0
		ladder = append(ladder, standing{cl, cl.reputation})
		cl.mu.Unlock()
	}
	slices.SortFunc(ladder, func(a, b standing) int {
		if c := cmp.Compare(b.reputation, a.reputation); c != 0 {
			return c
		}
		return cmp.Compare(a.cl.id, b.cl.id)
	})
	rank := 1
	for _, s := range ladder[:min(len(ladder), ladderSize)] {
		if s.reputation <= 0 {
			break
		}
		s.cl.mu.Lock()
		s.cl.rank = rank
		s.cl.mu.Unlock()
		rank++
	}
}
