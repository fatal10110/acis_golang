// Package basefunc provides the generic arithmetic building blocks the stat
// calculation chain composes: set/add/subtract/multiply/divide a Stat's
// running value, an enchant-level bonus, and the ordering contract that
// makes a chain of them deterministic regardless of attach order.
//
// A Func never resolves its own effector/effected/skill data — those are
// opaque to every op here except through its optional Condition gate. Ops
// that need real combat data (attribute-driven attack/defense/regen
// modifiers) are a different package built on top of this one.
//
// Not covered here: a reflection-style factory for building a Func from a
// name string and attaching it with a per-attachment owner/value/condition.
// That's XML-skill/item-data wiring, not arithmetic, and belongs with
// whichever loader ends up parsing skill/item function definitions.
package basefunc

import "github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"

// Order values group Funcs into calculation phases; a Calculator runs lower
// orders first and funcs sharing an order in unspecified order relative to
// each other. The values themselves (not just their relative sequence) are
// part of the contract: OrderFinalize is the phase reserved for the
// attribute-driven attack/defense/regen modifiers built on top of this
// package.
const (
	OrderSet      = 0  // override the base (template) value entirely
	OrderBaseMul  = 1  // add a flat ratio of the base value
	OrderBaseAdd  = 2  // add a flat amount to the base value
	OrderEnchant  = 3  // add an enchant-level-driven amount
	OrderFinalize = 10 // finalize the base value prior to further ops
	OrderMulDiv   = 20 // multiply/divide the running value
	OrderAddSub   = 30 // add/subtract the running value
	OrderAddMul   = 40 // multiply the running value by a percentage
)

// Condition gates whether a Func's calculation applies. effector, effected
// and skill are opaque here (any concrete data they need belongs to the
// condition implementation, not to this package) — matching how a Func
// itself never inspects them beyond passing them through.
type Condition interface {
	Test(effector, effected, skill any) bool
}

// Func is one node of a calculation chain: given the value computed so far
// (and the base value calculation started from), it returns the next
// value. Owner identifies whatever attached this Func (an item instance, a
// skill, …) so a Calculator can later remove every Func a given owner
// attached; it is opaque to this package.
type Func interface {
	Calc(effector, effected, skill any, base, value float64) float64
	Stat() stat.Stat
	Order() int
	Owner() any
	Value() float64
	Cond() Condition
}

// base is the embeddable state every concrete Func in this package shares:
// which Stat it targets, its calculation-order phase, the owner it was
// attached for, its configured value, and its optional gating Condition.
type base struct {
	owner any
	stat  stat.Stat
	order int
	value float64
	cond  Condition
}

func (b base) Stat() stat.Stat { return b.stat }
func (b base) Order() int      { return b.order }
func (b base) Owner() any      { return b.owner }
func (b base) Value() float64  { return b.value }
func (b base) Cond() Condition { return b.cond }

// passes reports whether b's Condition (if any) allows the calculation to
// proceed; an absent Condition always passes.
func (b base) passes(effector, effected, skill any) bool {
	return b.cond == nil || b.cond.Test(effector, effected, skill)
}
