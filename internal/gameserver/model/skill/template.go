package skill

import "fmt"

// Condition is one node of a skill condition tree. Kind is the element
// name, lowercased; Attrs are XML attributes after level-table resolution.
type Condition struct {
	Kind     string
	Attrs    map[string]string
	Children []Condition
}

// ConditionClause is a top-level condition block attached to a skill or a
// template group. Message and MessageID carry the optional failure feedback.
type ConditionClause struct {
	Root      Condition
	Message   string
	MessageID int32
	AddName   bool
}

// FuncOp is the arithmetic operation a stat template applies.
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

// ParseFuncOp resolves a stat-template element name to its operation.
func ParseFuncOp(tag string) (FuncOp, error) {
	switch tag {
	case "add", "ADD":
		return FuncAdd, nil
	case "addMul", "addmul", "ADDMUL":
		return FuncAddMul, nil
	case "sub", "SUB":
		return FuncSub, nil
	case "subDiv", "subdiv", "SUBDIV":
		return FuncSubDiv, nil
	case "mul", "MUL":
		return FuncMul, nil
	case "basemul", "baseMul", "BASEMUL":
		return FuncBaseMul, nil
	case "div", "DIV":
		return FuncDiv, nil
	case "set", "SET":
		return FuncSet, nil
	case "enchant", "ENCHANT":
		return FuncEnchant, nil
	case "baseadd", "baseAdd", "BASEADD":
		return FuncBaseAdd, nil
	default:
		return 0, fmt.Errorf("skill: unknown stat template element %q", tag)
	}
}

// FuncTemplate is one stat function attached by a skill/effect <for> block.
type FuncTemplate struct {
	Op              FuncOp
	Stat            string
	Value           float64
	AttachCondition *ConditionClause
	Condition       *Condition
}

// EffectTemplate is one parsed <effect> block. Runtime effect behavior is
// built later; this type preserves the full XML template shape.
type EffectTemplate struct {
	Name             string
	Value            float64
	Count            int
	Time             int
	Self             bool
	Icon             bool
	Abnormal         string
	StackType        string
	StackOrder       float64
	EffectPower      float64
	EffectType       string
	TriggeredID      int
	TriggeredLevel   int
	ChanceType       string
	ActivationChance int
	AttachCondition  *ConditionClause
	Funcs            []FuncTemplate
}
