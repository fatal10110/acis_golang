package conditions

// And is satisfied only when every child condition is. An empty And is
// vacuously satisfied.
type And struct {
	Conditions []Condition
}

func (c *And) Add(cond Condition) {
	if cond == nil {
		return
	}
	c.Conditions = append(c.Conditions, cond)
}

func (c *And) Test(effector, effected Actor, skill Skill) bool {
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
	Conditions []Condition
}

func (c *Or) Add(cond Condition) {
	if cond == nil {
		return
	}
	c.Conditions = append(c.Conditions, cond)
}

func (c *Or) Test(effector, effected Actor, skill Skill) bool {
	for _, cond := range c.Conditions {
		if cond.Test(effector, effected, skill) {
			return true
		}
	}
	return false
}

// Not inverts its single child condition.
type Not struct {
	Condition Condition
}

func (c Not) Test(effector, effected Actor, skill Skill) bool {
	return !c.Condition.Test(effector, effected, skill)
}
