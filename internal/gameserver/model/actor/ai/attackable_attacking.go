package ai

// AttackingNow reports whether the actor's attack is in flight: its swing
// started and its hit has not ended.
func (a *Attackable) AttackingNow() bool { return a.attack.AttackingNow() }
