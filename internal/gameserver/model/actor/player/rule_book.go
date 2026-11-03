package player

import (
	"sync/atomic"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
)

// RuleBook holds the Rules in force for every character configured with it.
// Set puts new Rules in force at once for all of them: each later read sees
// them whole, so one decision never mixes the rules before and after it.
type RuleBook struct {
	rules atomic.Pointer[Rules]
}

// NewRuleBook returns a RuleBook holding r.
func NewRuleBook(r Rules) *RuleBook {
	b := &RuleBook{}
	b.Set(r)
	return b
}

// Set puts r in force in place of the Rules before.
func (b *RuleBook) Set(r Rules) {
	b.rules.Store(&r)
}

// MaxBuffsAmount is the base buff-slot count in force.
func (b *RuleBook) MaxBuffsAmount() int {
	return b.rules.Load().MaxBuffsAmount
}

// RespawnRestoreHP is the share of max HP a revive without a Phoenix
// Blessing restores, in force.
func (b *RuleBook) RespawnRestoreHP() float64 {
	return b.rules.Load().RespawnRestoreHP
}

// RegenMultipliers are the base regeneration multipliers in force.
func (b *RuleBook) RegenMultipliers() creature.RegenMultipliers {
	return b.rules.Load().Regen.InForce()
}

// unconfiguredRules are the rules of a character not configured yet.
var unconfiguredRules = Rules{MaxBuffsAmount: defaultMaxBuffsAmount, FakeDeathUpProtection: DefaultFakeDeathUpProtection}

// DefaultFakeDeathUpProtection is the shipped post-Fake-Death aggro grace.
const DefaultFakeDeathUpProtection = 5 * time.Second

// rules returns the Rules in force for c. A caller reading several rules
// for one decision takes them from one call.
func (c *Character) rules() *Rules {
	if b := c.ruleBook.Load(); b != nil {
		return b.rules.Load()
	}
	return &unconfiguredRules
}
