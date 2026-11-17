package world

// RefreshKnown makes t and every object around the region holding it see
// each other afresh, as an instant jump that keeps t's region leaves them:
// region by region, each object that observes is told of t, and then t, when
// it observes, of that object. Nothing is forgotten and no region crossing
// is reported. It does nothing when t is off the grid.
//
// The callbacks run on the caller's goroutine with no world lock held,
// under the Observer rules.
func (s *State) RefreshKnown(t Tracked) {
	var buf [32]Tracked
	known := s.AppendKnown(buf[:0], t)
	tObs, tObserves := t.(Observer)
	for _, o := range known {
		if w, ok := o.(Observer); ok {
			w.Discover(t)
		}
		if tObserves {
			tObs.Discover(o)
		}
	}
}
