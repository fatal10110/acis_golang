package basefunc

import "github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"

// Set overrides the running value with its configured value outright (e.g.
// a weapon's flat P.Atk). It runs at OrderSet, first of every op.
type Set struct{ base }

// NewSet builds a Set targeting s with the given value, gated by the
// optional cond (nil for none).
func NewSet(owner any, s stat.Stat, value float64, cond Condition) Set {
	return Set{base{owner, s, OrderSet, value, cond}}
}

func (f Set) Calc(effector, effected, skill any, base, value float64) float64 {
	if !f.passes(effector, effected, skill) {
		return value
	}
	return f.value
}

// BaseMul adds a flat ratio of the base value to the running value (e.g. a
// skill's flat critical-chance bonus). It runs at OrderBaseMul.
type BaseMul struct{ base }

func NewBaseMul(owner any, s stat.Stat, value float64, cond Condition) BaseMul {
	return BaseMul{base{owner, s, OrderBaseMul, value, cond}}
}

func (f BaseMul) Calc(effector, effected, skill any, base, value float64) float64 {
	if !f.passes(effector, effected, skill) {
		return value
	}
	return value + base*f.value
}

// BaseAdd adds a flat amount to the running value (e.g. an item's P.Def or
// M.Def bonus). It runs at OrderBaseAdd.
type BaseAdd struct{ base }

func NewBaseAdd(owner any, s stat.Stat, value float64, cond Condition) BaseAdd {
	return BaseAdd{base{owner, s, OrderBaseAdd, value, cond}}
}

func (f BaseAdd) Calc(effector, effected, skill any, base, value float64) float64 {
	if !f.passes(effector, effected, skill) {
		return value
	}
	return value + f.value
}

// Mul multiplies the running value (e.g. a skill's percentage bonus). It
// runs at OrderMulDiv.
type Mul struct{ base }

func NewMul(owner any, s stat.Stat, value float64, cond Condition) Mul {
	return Mul{base{owner, s, OrderMulDiv, value, cond}}
}

func (f Mul) Calc(effector, effected, skill any, base, value float64) float64 {
	if !f.passes(effector, effected, skill) {
		return value
	}
	return value * f.value
}

// Div divides the running value. It runs at OrderMulDiv.
type Div struct{ base }

func NewDiv(owner any, s stat.Stat, value float64, cond Condition) Div {
	return Div{base{owner, s, OrderMulDiv, value, cond}}
}

func (f Div) Calc(effector, effected, skill any, base, value float64) float64 {
	if !f.passes(effector, effected, skill) {
		return value
	}
	return value / f.value
}

// Add adds a flat amount to the running value (e.g. a skill's flat bonus).
// It runs at OrderAddSub.
type Add struct{ base }

func NewAdd(owner any, s stat.Stat, value float64, cond Condition) Add {
	return Add{base{owner, s, OrderAddSub, value, cond}}
}

func (f Add) Calc(effector, effected, skill any, base, value float64) float64 {
	if !f.passes(effector, effected, skill) {
		return value
	}
	return value + f.value
}

// Sub subtracts a flat amount from the running value. It runs at
// OrderAddSub.
type Sub struct{ base }

func NewSub(owner any, s stat.Stat, value float64, cond Condition) Sub {
	return Sub{base{owner, s, OrderAddSub, value, cond}}
}

func (f Sub) Calc(effector, effected, skill any, base, value float64) float64 {
	if !f.passes(effector, effected, skill) {
		return value
	}
	return value - f.value
}

// AddMul reduces the running value by a percentage (e.g. a skill affecting
// a resistance). It runs at OrderAddMul, last of every op.
type AddMul struct{ base }

func NewAddMul(owner any, s stat.Stat, value float64, cond Condition) AddMul {
	return AddMul{base{owner, s, OrderAddMul, value, cond}}
}

func (f AddMul) Calc(effector, effected, skill any, base, value float64) float64 {
	if !f.passes(effector, effected, skill) {
		return value
	}
	return value * (1 - f.value/100)
}

// SubDiv is AddMul's inverse: it divides the running value by the
// complement of a percentage. It runs at OrderAddMul.
type SubDiv struct{ base }

func NewSubDiv(owner any, s stat.Stat, value float64, cond Condition) SubDiv {
	return SubDiv{base{owner, s, OrderAddMul, value, cond}}
}

func (f SubDiv) Calc(effector, effected, skill any, base, value float64) float64 {
	if !f.passes(effector, effected, skill) {
		return value
	}
	return value / (1 - f.value/100)
}
