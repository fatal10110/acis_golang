package player

import "github.com/fatal10110/acis_golang/internal/gameserver/party"

// PartyRef names one party for as long as it exists, whoever joins, leaves
// or leads it meanwhile. Once the party is gone it reads as having no
// leader and no members. Compare two with Same, never with ==.
type PartyRef struct {
	social SocialGraph
	id     party.ID
}

// Party returns a reference to c's party; false when c is in none.
func (c *Character) Party() (PartyRef, bool) {
	if c.social == nil {
		return PartyRef{}, false
	}
	id, ok := c.social.PartyOf(c.ObjectID())
	if !ok {
		return PartyRef{}, false
	}
	return PartyRef{social: c.social, id: id}, true
}

// Same reports whether r and o name one party.
func (r PartyRef) Same(o PartyRef) bool { return r.id == o.id }

// Leader returns the party's leader; nil once the party is gone.
func (r PartyRef) Leader() *Character {
	leader, _ := r.view()
	return leader
}

// Members returns the party's characters, in party order; nil once the
// party is gone.
func (r PartyRef) Members() []*Character {
	_, members := r.view()
	return members
}

func (r PartyRef) view() (*Character, []*Character) {
	if r.social == nil {
		return nil, nil
	}
	leader, members, ok := r.social.PartyByID(r.id)
	if !ok {
		return nil, nil
	}
	return leader, members
}
