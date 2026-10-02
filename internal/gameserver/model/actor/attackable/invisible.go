package attackable

// invisibleCombatant is a combatant that can be hidden.
type invisibleCombatant interface {
	Invisible() bool
}

// HiddenActingPlayer reports whether target's acting player — target itself,
// or the owner of a summon — is invisible.
func HiddenActingPlayer(target Combatant) bool {
	if target == nil {
		return false
	}
	acting := target
	if owner, ok := target.Owner(); ok && owner != nil {
		acting = owner
	}
	hidden, ok := acting.(invisibleCombatant)
	return ok && hidden.Invisible()
}
