package player

// Party names one party for as long as it is held, whoever joins, leaves or
// leads it meanwhile: two name the same party exactly when they are equal.
// A disbanded party keeps the leader it last had and has no members.
type Party interface {
	// Leader returns the party's leader.
	Leader() *Character
	// Members returns the party's characters in join order.
	Members() []*Character
}

// Party returns c's party; false when c is in none.
func (c *Character) Party() (Party, bool) {
	if c.social == nil {
		return nil, false
	}
	return c.social.Party(c.ObjectID())
}
