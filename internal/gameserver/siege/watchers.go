package siege

import (
	"slices"

	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
)

// OnStatusChange has fn called with each status the siege changes to from
// now on, after the functions added before it. fn runs once the engine's
// lock is released, at the change's place among the change's other world
// effects, so it may read the siege back.
func (s *Siege) OnStatusChange(fn func(Status)) {
	s.e.mu.Lock()
	defer s.e.mu.Unlock()
	s.watchers = append(s.watchers, fn)
}

// changeStatusLocked moves the siege to st and queues the call of every
// watcher with it.
func (s *Siege) changeStatusLocked(fx *sim.Effects, st Status) {
	s.status = st
	if len(s.watchers) == 0 {
		return
	}
	watchers := slices.Clone(s.watchers)
	fx.Add(func() {
		for _, fn := range watchers {
			fn(st)
		}
	})
}
