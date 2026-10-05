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
	held, _ := Evaluate(c, effector, effected, skill)
	return held
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
	held, _ := Evaluate(c, effector, effected, skill)
	return held
}

// Not inverts its single child condition. A Not whose child is nil (a
// <not> around a node that holds no condition) cannot be tested: see
// Evaluate.
type Not struct {
	Condition Condition
}

func (c Not) Test(effector, effected Actor, skill Skill) bool {
	held, _ := Evaluate(c, effector, effected, skill)
	return held
}

// Evaluate tests c and reports aborted when the test reached a condition
// that cannot be tested: c itself is nil, or And/Or evaluation (which stops
// at the first child that decides the result) reached a Not with a nil
// child. An aborted test does not hold, and the caller drops the feedback
// the condition would have given (see EvaluateSkill). Test on And, Or and
// Not is Evaluate without the abort report.
func Evaluate(c Condition, effector, effected Actor, skill Skill) (held, aborted bool) {
	switch c := c.(type) {
	case nil:
		return false, true
	case *And:
		for _, child := range c.Conditions {
			if held, aborted := Evaluate(child, effector, effected, skill); !held {
				return false, aborted
			}
		}
		return true, false
	case *Or:
		for _, child := range c.Conditions {
			held, aborted := Evaluate(child, effector, effected, skill)
			if held || aborted {
				return held, aborted
			}
		}
		return false, false
	case Not:
		held, aborted := Evaluate(c.Condition, effector, effected, skill)
		if aborted {
			return false, true
		}
		return !held, false
	default:
		return c.Test(effector, effected, skill), false
	}
}
