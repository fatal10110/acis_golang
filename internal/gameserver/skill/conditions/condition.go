package conditions

// The failure-report data a condition tree can carry (a literal message or
// a system-message id, optionally with the failing item/skill's name
// interpolated in) belongs to the tree's root, one per <cond> block — see
// model/item.UseCondition, which already owns it. Nothing in this package
// duplicates that.

// Condition is this package's own cast-time gate contract, decoupled from
// effect.Condition: a stat function is gated by its owner alone, while a
// cast condition needs both sides of the cast plus the skill being cast.
type Condition interface {
	Test(effector, effected Actor, skill Skill) bool
}

// asPlayer asserts effector to PlayerActor, reporting ok=false rather than
// panicking. A non-player effector is a normal condition result: most
// player-only conditions simply fail instead of treating it as a wiring bug.
func asPlayer(effector Actor) (PlayerActor, bool) {
	p, ok := effector.(PlayerActor)
	return p, ok
}
