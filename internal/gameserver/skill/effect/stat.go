package effect

import (
	"errors"
	"fmt"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// ErrEnchantNeedsItem is the expected skip when a skill's func templates
// include an enchant op and the owner is not an item. NPC and summon
// template passives have no item owner.
var ErrEnchantNeedsItem = errors.New("enchant stat funcs need an item owner")

func statFuncs(owner ModOwner, templates []modelskill.FuncTemplate, cond Condition) ([]Mod, error) {
	mods := make([]Mod, 0, len(templates))
	for _, tmpl := range templates {
		gate, err := funcCondition(tmpl.Condition, tmpl.AttachCondition)
		if err != nil {
			return nil, err
		}
		modCond := andCond(cond, gate)
		s, err := stat.ByName(tmpl.Stat)
		if err != nil {
			return nil, err
		}
		m, err := statFunc(owner, s, tmpl, modCond)
		if err != nil {
			return nil, err
		}
		mods = append(mods, m)
	}
	return mods, nil
}

func statFunc(owner ModOwner, s stat.Stat, tmpl modelskill.FuncTemplate, cond Condition) (Mod, error) {
	op, err := statOp(tmpl.Op)
	if err != nil {
		return Mod{}, err
	}
	return Mod{Stat: s, Op: op, Value: tmpl.Value, Cond: cond, Owner: owner}, nil
}

func statOp(op modelskill.FuncOp) (Op, error) {
	switch op {
	case modelskill.FuncAdd:
		return OpAdd, nil
	case modelskill.FuncAddMul:
		return OpAddMul, nil
	case modelskill.FuncSub:
		return OpSub, nil
	case modelskill.FuncSubDiv:
		return OpSubDiv, nil
	case modelskill.FuncMul:
		return OpMul, nil
	case modelskill.FuncBaseMul:
		return OpBaseMul, nil
	case modelskill.FuncDiv:
		return OpDiv, nil
	case modelskill.FuncSet:
		return OpSet, nil
	case modelskill.FuncBaseAdd:
		return OpBaseAdd, nil
	case modelskill.FuncEnchant:
		return 0, ErrEnchantNeedsItem
	default:
		return 0, fmt.Errorf("unknown stat func op %s", op)
	}
}

// DamageOverTimeInput is the state a periodic HP damage tick needs.
