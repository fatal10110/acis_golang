package party

// LootOrigin is where loot came from: a member shares it only within party
// range of it, body to body.
type LootOrigin = Body

// Looter picks the member who takes an item pickerID looted, by its
// party's loot rule. Finders-keepers leaves it with the picker, as do the
// random and by-turn rules for a spoil they do not include; otherwise a
// random member, or the next one in turn, that eligible accepts takes it.
// When no member is eligible the picker keeps it. members is the party in
// join order, for the notices the other members get. ok is false when
// pickerID is in no party.
//
// eligible runs without the registry lock held, so it may take the
// members' own locks.
func (r *Registry[M]) Looter(pickerID int32, spoil bool, eligible func(M) bool) (looter M, members []M, ok bool) {
	r.mu.Lock()
	g := r.byMember[pickerID]
	if g == nil {
		r.mu.Unlock()
		return looter, nil, false
	}
	members = append([]M(nil), g.members...)
	rule := g.loot
	r.mu.Unlock()

	for _, m := range members {
		if m.ObjectID() == pickerID {
			looter = m
		}
	}
	turn := false
	switch rule {
	case LootRandom, LootByTurn:
		if spoil {
			return looter, members, true
		}
		turn = rule == LootByTurn
	case LootRandomIncludingSpoil:
	case LootByTurnIncludingSpoil:
		turn = true
	default:
		return looter, members, true
	}

	valid := make(map[int32]bool, len(members))
	var candidates []M
	for _, m := range members {
		if eligible(m) {
			valid[m.ObjectID()] = true
			candidates = append(candidates, m)
		}
	}
	if turn {
		if next, found := r.nextLooter(g, valid); found {
			looter = next
		}
		return looter, members, true
	}
	if len(candidates) > 0 {
		looter = candidates[r.intn(len(candidates))]
	}
	return looter, members, true
}

// nextLooter advances g's turn to the next member in valid, trying every
// member once; the turn ends on the member found, or back where it started
// when there is none.
func (r *Registry[M]) nextLooter(g *group[M], valid map[int32]bool) (M, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := len(g.members)
	for range n {
		g.lastLoot++
		if g.lastLoot >= n {
			g.lastLoot = 0
		}
		if m := g.members[g.lastLoot]; valid[m.ObjectID()] {
			return m, true
		}
	}
	var zero M
	return zero, false
}
