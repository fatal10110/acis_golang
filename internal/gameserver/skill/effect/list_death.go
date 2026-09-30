package effect

// StopOnDeath removes the effects a playable loses when it dies. A Phoenix
// Blessing or a Noblesse Blessing keeps every other effect and only strips
// the blessings themselves (Charm of Luck with them); otherwise every effect
// that does not last through death is removed.
//
// blessingStops counts the blessing and charm stops that ran: each one is a
// stop by type the owner follows with its own appearance refresh, on top of
// the refresh every removed effect already triggers. strippedAll reports
// that the stop-all ran instead: its removals announced no stat change, so
// the owner refreshes its view once.
func (l *List) StopOnDeath() (blessingStops int, strippedAll bool) {
	if l == nil {
		return 0, false
	}
	has := func(want Type) bool {
		for _, e := range l.All() {
			if e.Type == want {
				return true
			}
		}
		return false
	}
	stop := func(t Type) {
		if has(t) {
			l.StopByType(t)
			blessingStops++
		}
	}
	if has(TypePhoenixBless) {
		stop(TypeCharmOfLuck)
		stop(TypeNoblesseBless)
		return blessingStops, false
	}
	if has(TypeNoblesseBless) {
		stop(TypeNoblesseBless)
		stop(TypeCharmOfLuck)
		return blessingStops, false
	}
	l.StopAllExceptThoseThatLastThroughDeath()
	return 0, true
}
