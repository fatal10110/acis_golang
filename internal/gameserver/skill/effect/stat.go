package effect

import (
	"fmt"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/basefunc"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

func statFuncs(owner any, templates []modelskill.FuncTemplate, cond basefunc.Condition) ([]basefunc.Func, error) {
	funcs := make([]basefunc.Func, 0, len(templates))
	for _, tmpl := range templates {
		if tmpl.AttachCondition != nil || tmpl.Condition != nil {
			return nil, fmt.Errorf("conditional stat funcs are not wired yet")
		}
		s, err := stat.ByName(tmpl.Stat)
		if err != nil {
			return nil, err
		}
		fn, err := statFunc(owner, s, tmpl, cond)
		if err != nil {
			return nil, err
		}
		funcs = append(funcs, fn)
	}
	return funcs, nil
}

func statFunc(owner any, s stat.Stat, tmpl modelskill.FuncTemplate, cond basefunc.Condition) (basefunc.Func, error) {
	switch tmpl.Op {
	case modelskill.FuncAdd:
		return basefunc.NewAdd(owner, s, tmpl.Value, cond), nil
	case modelskill.FuncAddMul:
		return basefunc.NewAddMul(owner, s, tmpl.Value, cond), nil
	case modelskill.FuncSub:
		return basefunc.NewSub(owner, s, tmpl.Value, cond), nil
	case modelskill.FuncSubDiv:
		return basefunc.NewSubDiv(owner, s, tmpl.Value, cond), nil
	case modelskill.FuncMul:
		return basefunc.NewMul(owner, s, tmpl.Value, cond), nil
	case modelskill.FuncBaseMul:
		return basefunc.NewBaseMul(owner, s, tmpl.Value, cond), nil
	case modelskill.FuncDiv:
		return basefunc.NewDiv(owner, s, tmpl.Value, cond), nil
	case modelskill.FuncSet:
		return basefunc.NewSet(owner, s, tmpl.Value, cond), nil
	case modelskill.FuncBaseAdd:
		return basefunc.NewBaseAdd(owner, s, tmpl.Value, cond), nil
	case modelskill.FuncEnchant:
		return nil, fmt.Errorf("enchant stat funcs need an item owner")
	default:
		return nil, fmt.Errorf("unknown stat func op %s", tmpl.Op)
	}
}

// DamageOverTimeInput is the state a periodic HP damage tick needs.
