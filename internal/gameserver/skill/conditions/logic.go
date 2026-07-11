package conditions

import "github.com/fatal10110/acis_golang/internal/gameserver/skill/basefunc"

// And is satisfied only when every child condition is. An empty And is
// vacuously satisfied.
type And struct {
	Conditions []basefunc.Condition
}

func (c *And) Add(cond basefunc.Condition) {
	if cond == nil {
		return
	}
	c.Conditions = append(c.Conditions, cond)
}

func (c *And) Test(effector, effected, skill any) bool {
	for _, cond := range c.Conditions {
		if !cond.Test(effector, effected, skill) {
			return false
		}
	}
	return true
}

// Or is satisfied when any child condition is. An empty Or is never
// satisfied.
type Or struct {
	Conditions []basefunc.Condition
}

func (c *Or) Add(cond basefunc.Condition) {
	if cond == nil {
		return
	}
	c.Conditions = append(c.Conditions, cond)
}

func (c *Or) Test(effector, effected, skill any) bool {
	for _, cond := range c.Conditions {
		if cond.Test(effector, effected, skill) {
			return true
		}
	}
	return false
}

// Not inverts its single child condition.
type Not struct {
	Condition basefunc.Condition
}

func (c Not) Test(effector, effected, skill any) bool {
	return !c.Condition.Test(effector, effected, skill)
}
