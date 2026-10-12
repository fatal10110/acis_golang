package zone

// Holders maps the object id of each occupant of ix's zones to the zones
// holding it, in load order.
func (ix *Index) Holders() map[int32][]Kind {
	held := map[int32][]Kind{}
	for _, k := range ix.All() {
		for _, a := range k.Core().Occupants() {
			held[a.ObjectID()] = append(held[a.ObjectID()], k)
		}
	}
	return held
}

// Clear drops every occupant of ix's zones without the zones' exit rules.
func (ix *Index) Clear() {
	for _, k := range ix.All() {
		z := k.Core()
		z.mu.Lock()
		clear(z.occupants)
		z.mu.Unlock()
	}
}

// Rebase moves a off held, the zones of a swapped-out index that hold it,
// and onto the zones of ix at its position, as a zone reload does: a is
// dropped from held without their exit rules and loses every flag they
// raised, then, when it stands on the grid (onGrid), enters the zones of ix
// around it, their entry rules and watchers running as on any entry. A
// swimmer or wader that only the dropped zones kept in water or swamp is
// told it left, and so is a player that only they kept in danger. The caller holds a's membership, as for Revalidate.
func Rebase(ix *Index, held []Kind, a Actor, onGrid bool) {
	for _, k := range held {
		k.Core().evict(a)
	}
	a.ZoneFlags().Reset()
	if onGrid && ix != nil {
		ix.Revalidate(a)
	}
	var water, danger bool
	var swamp *Swamp
	for _, k := range held {
		switch z := k.(type) {
		case *Water:
			water = true
		case *Swamp:
			swamp = z
		case *Damage, *Effect:
			danger = a.Class() == ClassPlayer
		}
	}
	if s, ok := a.(Swimmer); ok && water && !a.ZoneFlags().Has(FlagWater) {
		s.SwimStateChanged(false)
	}
	if w, ok := a.(Wader); ok && swamp != nil && !a.ZoneFlags().Has(FlagSwamp) {
		w.SwampStateChanged(swamp)
	}
	if e, ok := a.(Endangered); ok && danger && !a.ZoneFlags().Has(FlagDanger) {
		e.DangerStateChanged()
	}
}

// Reload moves a off held, the zones of a swapped-out index that hold it,
// and onto those of ix at its position (Rebase). A creature off the grid or
// mid-teleport enters none until it lands.
func (m *Member) Reload(ix *Index, a Actor, held []Kind) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.steps = 0
	Rebase(ix, held, a, m.onGrid && !teleporting(a))
}
