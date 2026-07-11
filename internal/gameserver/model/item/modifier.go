package item

import (
	"fmt"
	"strings"

	"github.com/fatal10110/acis_golang/internal/commons"
)

// FuncOp is the arithmetic operation one stat modifier applies to the stat
// it names.
type FuncOp uint8

const (
	FuncAdd FuncOp = iota
	FuncAddMul
	FuncSub
	FuncSubDiv
	FuncMul
	FuncBaseMul
	FuncDiv
	FuncSet
	FuncEnchant
	FuncBaseAdd
)

// String returns the canonical XML element spelling for op.
func (op FuncOp) String() string {
	switch op {
	case FuncAdd:
		return "add"
	case FuncAddMul:
		return "addMul"
	case FuncSub:
		return "sub"
	case FuncSubDiv:
		return "subDiv"
	case FuncMul:
		return "mul"
	case FuncBaseMul:
		return "basemul"
	case FuncDiv:
		return "div"
	case FuncSet:
		return "set"
	case FuncEnchant:
		return "enchant"
	case FuncBaseAdd:
		return "baseadd"
	default:
		return fmt.Sprintf("FuncOp(%d)", uint8(op))
	}
}

// funcOpNames maps a stat-modifier element's tag name, lowercased, to the
// FuncOp it selects.
var funcOpNames = map[string]FuncOp{
	"add":     FuncAdd,
	"addmul":  FuncAddMul,
	"sub":     FuncSub,
	"subdiv":  FuncSubDiv,
	"mul":     FuncMul,
	"basemul": FuncBaseMul,
	"div":     FuncDiv,
	"set":     FuncSet,
	"enchant": FuncEnchant,
	"baseadd": FuncBaseAdd,
}

// ParseFuncOp resolves a stat-modifier element's tag name to a FuncOp,
// matching case-insensitively. It returns an error for any other value
// rather than guessing.
func ParseFuncOp(tag string) (FuncOp, error) {
	op, ok := funcOpNames[strings.ToLower(tag)]
	if !ok {
		return 0, fmt.Errorf("item: unknown stat modifier element %q", tag)
	}
	return op, nil
}

// StatModifier is one bonus a template applies to whichever stat it names
// while equipped. Stat is the raw stat identifier as it appears in the data
// file: resolving it against the engine's stat catalog is that engine's
// job, not this loader's.
type StatModifier struct {
	Op              FuncOp
	Stat            string
	Value           float64
	AttachCondition *UseCondition
	Condition       *Condition
}

// NewStatModifier builds a StatModifier of the given op from set, the
// folded attributes of one stat-modifier element. "stat" and "val" are both
// required.
func NewStatModifier(op FuncOp, set *commons.StatSet) (StatModifier, error) {
	stat, err := set.GetString("stat")
	if err != nil {
		return StatModifier{}, fmt.Errorf("item: stat modifier: %w", err)
	}
	val, err := set.GetFloat64("val")
	if err != nil {
		return StatModifier{}, fmt.Errorf("item: stat modifier %q: %w", stat, err)
	}
	return StatModifier{Op: op, Stat: stat, Value: val}, nil
}
