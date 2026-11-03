package party

// PartyRef names one party for as long as it is held, even once the party
// disbands: two refs are equal exactly when they name the same party.
type PartyRef[M Member] struct {
	r *Registry[M]
	g *group[M]
}

// PartyRef returns memberID's party.
func (r *Registry[M]) PartyRef(memberID int32) (PartyRef[M], bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	g := r.byMember[memberID]
	if g == nil {
		return PartyRef[M]{}, false
	}
	return PartyRef[M]{r: r, g: g}, true
}

// Leader returns the party's current leader; a disbanded party keeps the
// one it last had.
func (ref PartyRef[M]) Leader() M {
	ref.r.mu.Lock()
	defer ref.r.mu.Unlock()
	return ref.g.leader
}

// Members returns the party's members in join order; a disbanded party has
// none.
func (ref PartyRef[M]) Members() []M {
	ref.r.mu.Lock()
	defer ref.r.mu.Unlock()
	return append([]M(nil), ref.g.members...)
}
