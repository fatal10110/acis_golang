package zone

import "github.com/fatal10110/acis_golang/internal/gameserver/model/location"

// holders maps the object id of each occupant of s's zones to the zones
// holding it, in load order. It is taken once, the first time it is asked
// for, which happens only after s was swapped out.
func (s *zoneSet) holders() map[int32][]Kind {
	s.retire.Do(func() {
		s.held = map[int32][]Kind{}
		for _, k := range s.all {
			for _, a := range k.Core().Occupants() {
				s.held[a.ObjectID()] = append(s.held[a.ObjectID()], k)
			}
		}
	})
	return s.held
}

// sync makes s, the zones of a's index now, the set a's zone holds are
// raised against. When a was last synced against a set a reload has since
// swapped out, a is dropped from that set's zones that held it, without
// their exit rules, and loses every flag; those zones are returned for
// settleDropped. The caller holds a's membership, as for Revalidate.
func (s *zoneSet) sync(a Actor) []Kind {
	prev := a.ZoneFlags().resync(s)
	if prev == nil {
		return nil
	}
	held := prev.holders()[a.ObjectID()]
	for _, k := range held {
		k.Core().evict(a)
	}
	return held
}

// settleDropped tells a, once it entered the zones around it, of the water,
// swamp or danger that only dropped, the zones sync took it out of, kept it
// in.
func settleDropped(a Actor, dropped []Kind) {
	if len(dropped) == 0 {
		return
	}
	var water, danger bool
	var swamp *Swamp
	for _, k := range dropped {
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

// Clear drops every occupant of ix's zones without the zones' exit rules.
// It runs on an index Swap has swapped out, once every creature was
// rebased; a creature it missed still leaves the zones that held it on its
// next revalidation.
func (ix *Index) Clear() {
	set := ix.set()
	set.holders()
	for _, k := range set.all {
		z := k.Core()
		z.mu.Lock()
		clear(z.occupants)
		z.mu.Unlock()
	}
}

// Rebase moves a onto the zones ix holds now, as a zone reload does. When a
// was last synced against zones the reload swapped out, it is dropped from
// those that held it without their exit rules and loses every flag they
// raised; then, when it stands on the grid (onGrid), it is revalidated
// against the zones of ix attached to the region containing held, the
// region holding it (see RevalidateIn), their entry rules and watchers
// running as on any entry. A creature an instant teleport moved out of that
// region so enters none of the zones at its landing point until its next
// move.
// A swimmer or wader that only the dropped zones kept in water or swamp is
// told it left, and so is a player that only they kept in danger.
//
// Any index operation against the new zones moves a the same way first, so
// a creature that moved before the reload reached it keeps the holds the
// new zones raised. The caller holds a's membership, as for Revalidate.
func (ix *Index) Rebase(a Actor, held location.Location, onGrid bool) {
	set := ix.set()
	dropped := set.sync(a)
	if onGrid {
		for _, k := range set.at(held.X, held.Y) {
			Revalidate(k, a)
		}
	}
	settleDropped(a, dropped)
}

// Reload moves a onto the zones ix holds now (Rebase), those of the region
// holding it. A creature off the grid or mid-teleport enters none until it
// lands.
func (m *Member) Reload(ix *Index, a Actor) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.steps = 0
	ix.Rebase(a, m.held, m.onGrid && !teleporting(a))
}
