package conditions

// The failure-report data a condition tree can carry (a literal message or
// a system-message id, optionally with the failing item/skill's name
// interpolated in) belongs to the tree's root, one per <cond> block — see
// model/item.UseCondition, which already owns it. Nothing in this package
// duplicates that.

// asPlayer asserts effector to PlayerActor, reporting ok=false (rather than
// panicking) when it doesn't — the Go equivalent of the reference
// implementation's "instanceof Player player" pattern, which is itself a
// normal, expected outcome (most player-only conditions simply fail for a
// non-player effector) rather than a wiring bug.
func asPlayer(effector any) (PlayerActor, bool) {
	p, ok := effector.(PlayerActor)
	return p, ok
}
