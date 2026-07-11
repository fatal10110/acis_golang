// Package skill contains skill rules and effects.
package skill

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/basefunc"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// Calculator dynamically computes the effect of a Stat: a slice of
// basefunc.Func kept sorted by Order, lowest first, with same-order Funcs
// applied in unspecified relative order (matching the reference chain,
// which offers no ordering guarantee among equal-order entries either).
//
// A Calculator's zero value is ready to use as an empty chain.
type Calculator struct {
	funcs []basefunc.Func
}

// Size returns the number of Funcs currently attached.
func (c *Calculator) Size() int { return len(c.funcs) }

// AddFunc inserts fn into the chain, keeping it sorted by Order: fn lands
// after every existing Func whose Order is <= fn.Order(), preserving the
// relative order of Funcs already at that Order.
func (c *Calculator) AddFunc(fn basefunc.Func) {
	order := fn.Order()
	i := 0
	for i < len(c.funcs) && order >= c.funcs[i].Order() {
		i++
	}
	c.funcs = append(c.funcs, nil)
	copy(c.funcs[i+1:], c.funcs[i:])
	c.funcs[i] = fn
}

// RemoveFunc removes fn from the chain. It is a no-op if fn is not present.
func (c *Calculator) RemoveFunc(fn basefunc.Func) {
	for i, f := range c.funcs {
		if f == fn {
			c.funcs = append(c.funcs[:i], c.funcs[i+1:]...)
			return
		}
	}
}

// RemoveOwner removes every Func whose Owner equals owner, returning the
// Stat each removed Func targeted.
func (c *Calculator) RemoveOwner(owner any) []stat.Stat {
	var modified []stat.Stat
	kept := c.funcs[:0]
	for _, f := range c.funcs {
		if f.Owner() == owner {
			modified = append(modified, f.Stat())
			continue
		}
		kept = append(kept, f)
	}
	c.funcs = kept
	return modified
}

// Calc runs every Func in the chain in order, starting the running value
// from base. A basefunc.Set overrides the base value seen by every later
// Func, mirroring how a template override (e.g. a weapon's flat P.Atk)
// replaces rather than augments the starting point for what follows.
func (c *Calculator) Calc(effector, effected, skill any, base float64) float64 {
	value := base
	for _, f := range c.funcs {
		value = f.Calc(effector, effected, skill, base, value)
		if _, ok := f.(basefunc.Set); ok {
			base = value
		}
	}
	return value
}
