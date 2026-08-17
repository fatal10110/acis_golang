package effect

import (
	"sync"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/funcs"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// Op identifies one data-driven stat modifier's arithmetic, parsed from an
// XML FuncTemplate (skill) or StatModifier (item) op attribute.
type Op int

const (
	OpSet     Op = iota // override the base (template) value entirely
	OpBaseMul           // add a flat ratio of the base value
	OpBaseAdd           // add a flat amount to the base value
	OpEnchant           // add an enchant-level-driven amount
	OpMul               // multiply the running value
	OpDiv               // divide the running value
	OpAdd               // add a flat amount to the running value
	OpSub               // subtract a flat amount from the running value
	OpAddMul            // reduce the running value by a percentage
	OpSubDiv            // AddMul's inverse: divide by the percentage complement
)

// Calculation-order phases a Calculator runs in, lowest first. The values
// themselves (not just their relative sequence) are part of the contract:
// orderFinalize is the phase reserved exclusively for the static,
// attribute-driven builtin pipeline (skill/funcs) — no data Mod ever
// occupies it.
const (
	orderSet      = 0
	orderBaseMul  = 1
	orderBaseAdd  = 2
	orderEnchant  = 3
	orderFinalize = 10
	orderMulDiv   = 20
	orderAddSub   = 30
	orderAddMul   = 40
)

// order returns op's calculation-order phase.
func (op Op) order() int {
	switch op {
	case OpSet:
		return orderSet
	case OpBaseMul:
		return orderBaseMul
	case OpBaseAdd:
		return orderBaseAdd
	case OpEnchant:
		return orderEnchant
	case OpMul, OpDiv:
		return orderMulDiv
	case OpAdd, OpSub:
		return orderAddSub
	default: // OpAddMul, OpSubDiv
		return orderAddMul
	}
}

// Condition gates whether a Mod's calculation applies against effector, the
// only side of a stat calculation a Mod ever reads.
type Condition interface {
	Test(effector stat.Actor) bool
}

// ModOwner identifies whatever attached a Mod, so a Calculator can later
// remove every Mod a given owner attached. It is a closed, comparable sum
// of the three things a Mod is ever attributed to: a running buff/debuff
// effect, a learned passive skill, or an equipped item. The zero ModOwner
// never equals a real owner (a nil *Effect, a zero modelskill.Ref, and a
// zero ItemOwner are all impossible for a genuinely attached Mod), so it
// is safe as a "no owner" placeholder without a separate flag.
type ModOwner struct {
	effect *Effect
	skill  modelskill.Ref
	item   ItemOwner
}

// ModOwnerEffect identifies a Mod attached by a running buff/debuff effect.
func ModOwnerEffect(e *Effect) ModOwner { return ModOwner{effect: e} }

// ModOwnerSkill identifies a Mod attached by a learned passive skill.
func ModOwnerSkill(ref modelskill.Ref) ModOwner { return ModOwner{skill: ref} }

// ModOwnerItem identifies a Mod attached by an equipped item instance.
func ModOwnerItem(owner ItemOwner) ModOwner { return ModOwner{item: owner} }

// Mod is one data-driven stat modifier: a single (stat, op, value,
// condition, owner) tuple with no behavior of its own beyond the pure
// arithmetic Op names. It replaces the former Func interface hierarchy —
// there was no polymorphism left to express once a modifier's owner became
// a typed value instead of a method on an interface implementation.
type Mod struct {
	Stat  stat.Stat
	Op    Op
	Value float64
	Cond  Condition // nil always applies
	Owner ModOwner
}

// apply runs m against the calculation chain's running value, gated by its
// Cond.
func apply(m Mod, actor stat.Actor, base, value float64) float64 {
	if m.Cond != nil && !m.Cond.Test(actor) {
		return value
	}
	switch m.Op {
	case OpSet:
		return m.Value
	case OpBaseMul:
		return value + base*m.Value
	case OpBaseAdd:
		return value + m.Value
	case OpEnchant:
		return applyEnchant(m, value)
	case OpMul:
		return value * m.Value
	case OpDiv:
		return value / m.Value
	case OpAdd:
		return value + m.Value
	case OpSub:
		return value - m.Value
	case OpAddMul:
		return value * (1 - m.Value/100)
	case OpSubDiv:
		return value / (1 - m.Value/100)
	default:
		return value
	}
}

// applyEnchant adds an amount driven by m.Owner's item's live enchant
// level: a flat per-level bonus to P.Def/M.Def, a crystal-grade-scaled
// bonus to M.Atk, or a crystal-grade-and-weapon-type-scaled bonus to P.Atk
// for weapons. m.Value is unused — the amount is entirely item-driven.
// m.Owner must be an item owner (see ModOwnerItem); building an Enchant Mod
// with any other owner is an itemStatFunc/statFunc construction bug, not a
// runtime state this function needs to defend against beyond a zero
// enchant level reading as "no bonus".
func applyEnchant(m Mod, value float64) float64 {
	src := m.Owner.item

	enchant := src.EnchantLevel()
	if enchant <= 0 {
		return value
	}

	overenchant := 0
	if enchant > 3 {
		overenchant = enchant - 3
		enchant = 3
	}

	if m.Stat == stat.MagicDefence || m.Stat == stat.PowerDefence {
		return value + float64(enchant) + float64(3*overenchant)
	}

	if m.Stat == stat.MagicAttack {
		switch src.Crystal() {
		case item.CrystalS:
			value += float64(4*enchant + 8*overenchant)
		case item.CrystalA, item.CrystalB, item.CrystalC:
			value += float64(3*enchant + 6*overenchant)
		case item.CrystalD:
			value += float64(2*enchant + 4*overenchant)
		}
		return value
	}

	wType, isWeapon := src.Weapon()
	if !isWeapon {
		return value
	}

	isBigOrDual := wType == item.WeaponBigBlunt || wType == item.WeaponBigSword || wType == item.WeaponDualFist || wType == item.WeaponDual

	switch src.Crystal() {
	case item.CrystalS:
		switch {
		case wType == item.WeaponBow:
			value += float64(10*enchant + 20*overenchant)
		case isBigOrDual:
			value += float64(6*enchant + 12*overenchant)
		default:
			value += float64(5*enchant + 10*overenchant)
		}
	case item.CrystalA:
		switch {
		case wType == item.WeaponBow:
			value += float64(8*enchant + 16*overenchant)
		case isBigOrDual:
			value += float64(5*enchant + 10*overenchant)
		default:
			value += float64(4*enchant + 8*overenchant)
		}
	case item.CrystalB, item.CrystalC:
		switch {
		case wType == item.WeaponBow:
			value += float64(6*enchant + 12*overenchant)
		case isBigOrDual:
			value += float64(4*enchant + 8*overenchant)
		default:
			value += float64(3*enchant + 6*overenchant)
		}
	case item.CrystalD:
		switch {
		case wType == item.WeaponBow:
			value += float64(4*enchant + 8*overenchant)
		default:
			value += float64(2*enchant + 4*overenchant)
		}
	}

	return value
}

// Calculator dynamically computes the effect of one Stat: a slice of data
// Mods kept sorted by Op's order (lowest first, same-order Mods in stable
// insertion order — float addition is not associative, so that insertion
// sequence is observable and load-bearing), plus a static builtin
// finalize step (see skill/funcs) that always runs at order 10, between
// Mods below and above that order. A builtin is attached once at
// construction and is never removed: unlike a Mod it carries no owner.
//
// mu guards mods; mutators publish fresh slices so Calc can use a stable
// snapshot while another goroutine attaches or detaches Mods. A Calculator
// must not be copied after first use.
type Calculator struct {
	mu      sync.RWMutex
	mods    []Mod
	builtin funcs.Func
}

// NewCalculator returns a Calculator whose static finalize step is
// builtin (nil for a Stat with no builtin).
func NewCalculator(builtin funcs.Func) Calculator {
	return Calculator{builtin: builtin}
}

// Size returns the number of data Mods currently attached (excluding the
// static builtin, which isn't a Mod).
func (c *Calculator) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.mods)
}

// AddMod inserts m into the chain, keeping it sorted by Op's order: m
// lands after every existing Mod whose order is <= m's, preserving the
// relative order of Mods already at that order.
func (c *Calculator) AddMod(m Mod) {
	c.mu.Lock()
	defer c.mu.Unlock()

	order := m.Op.order()
	mods := c.mods
	i := 0
	for i < len(mods) && order >= mods[i].Op.order() {
		i++
	}

	next := make([]Mod, len(mods)+1)
	copy(next, mods[:i])
	next[i] = m
	copy(next[i+1:], mods[i:])
	c.mods = next
}

// RemoveOwner removes every Mod whose Owner equals owner.
func (c *Calculator) RemoveOwner(owner ModOwner) {
	c.mu.Lock()
	defer c.mu.Unlock()

	kept := make([]Mod, 0, len(c.mods))
	changed := false
	for _, m := range c.mods {
		if m.Owner == owner {
			changed = true
			continue
		}
		kept = append(kept, m)
	}
	if changed {
		c.mods = kept
	}
}

// Calc runs the chain for actor, starting the running value from base:
// every Mod ordered below the builtin finalize step, then the builtin (if
// any), then every Mod ordered above it. A Set overrides the base value
// seen by everything that runs after it, including the builtin, mirroring
// how a template override (e.g. a weapon's flat P.Atk) replaces rather
// than augments the starting point for what follows.
func (c *Calculator) Calc(actor stat.Actor, base float64) float64 {
	c.mu.RLock()
	mods := c.mods
	builtin := c.builtin
	c.mu.RUnlock()

	value := base
	i := 0
	for ; i < len(mods) && mods[i].Op.order() < orderFinalize; i++ {
		value = apply(mods[i], actor, base, value)
		if mods[i].Op == OpSet {
			base = value
		}
	}
	if builtin != nil {
		value = builtin(actor, base, value)
	}
	for ; i < len(mods); i++ {
		value = apply(mods[i], actor, base, value)
	}
	return value
}
