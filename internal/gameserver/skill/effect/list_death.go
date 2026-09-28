package effect

// StopOnDeath removes the effects a playable loses when it dies. A Phoenix
// Blessing or a Noblesse Blessing keeps every other effect and only strips
// the blessings themselves (Charm of Luck with them); otherwise every effect
// that does not last through death is removed.
func (l *List) StopOnDeath() {
	if l == nil {
		return
	}
	has := func(want Type) bool {
		for _, e := range l.All() {
			if e.Type == want {
				return true
			}
		}
		return false
	}
	if has(TypePhoenixBless) {
		l.StopByType(TypeCharmOfLuck)
		l.StopByType(TypeNoblesseBless)
		return
	}
	if has(TypeNoblesseBless) {
		l.StopByType(TypeNoblesseBless)
		l.StopByType(TypeCharmOfLuck)
		return
	}
	l.StopAllExceptThoseThatLastThroughDeath()
}
