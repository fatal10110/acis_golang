// Package skill contains skill rules and effects.
package skill

import (
	"sync"

	"github.com/fatal10110/acis_golang/internal/gameserver/skill/basefunc"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// Calculator dynamically computes the effect of a Stat: a slice of
// basefunc.Func kept sorted by Order, lowest first, with same-order Funcs
// applied in unspecified relative order.
//
// A Calculator's zero value is ready to use as an empty chain. mu guards
// funcs; mutators publish fresh slices so Calc can use a stable snapshot
// while another goroutine attaches or detaches funcs. A Calculator must
// not be copied after first use.
type Calculator struct {
	mu    sync.RWMutex
	funcs []basefunc.Func
}

// Size returns the number of Funcs currently attached.
func (c *Calculator) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.funcs)
}

// AddFunc inserts fn into the chain, keeping it sorted by Order: fn lands
// after every existing Func whose Order is <= fn.Order(), preserving the
// relative order of Funcs already at that Order.
func (c *Calculator) AddFunc(fn basefunc.Func) {
	c.mu.Lock()
	defer c.mu.Unlock()

	order := fn.Order()
	funcs := c.funcs
	i := 0
	for i < len(funcs) && order >= funcs[i].Order() {
		i++
	}

	next := make([]basefunc.Func, len(funcs)+1)
	copy(next, funcs[:i])
	next[i] = fn
	copy(next[i+1:], funcs[i:])
	c.funcs = next
}

// RemoveFunc removes fn from the chain. It is a no-op if fn is not present.
func (c *Calculator) RemoveFunc(fn basefunc.Func) {
	c.mu.Lock()
	defer c.mu.Unlock()

	funcs := c.funcs
	for i, f := range funcs {
		if f == fn {
			next := make([]basefunc.Func, len(funcs)-1)
			copy(next, funcs[:i])
			copy(next[i:], funcs[i+1:])
			c.funcs = next
			return
		}
	}
}

// RemoveOwner removes every Func whose Owner equals owner, returning the
// Stat each removed Func targeted.
func (c *Calculator) RemoveOwner(owner any) []stat.Stat {
	c.mu.Lock()
	defer c.mu.Unlock()

	var modified []stat.Stat
	kept := make([]basefunc.Func, 0, len(c.funcs))
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
	c.mu.RLock()
	funcs := c.funcs
	c.mu.RUnlock()

	value := base
	for _, f := range funcs {
		value = f.Calc(effector, effected, skill, base, value)
		if _, ok := f.(*basefunc.Set); ok {
			base = value
		}
	}
	return value
}
