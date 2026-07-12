package effect

import (
	"fmt"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/basefunc"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// Flag is the bitmask exposed by an active effect to live actor state.
type Flag uint32

const (
	// FlagNone is the default effect flag mask.
	FlagNone Flag = 1 << iota
	flagCharmOfCourage
	flagCharmOfLuck
	flagPhoenixBlessing
	flagNoblesseBlessing
	flagSilentMove
	flagProtectionBlessing
	flagRelaxing
	// FlagFear marks a target as feared.
	FlagFear
	flagConfused
	flagMuted
	flagPhysicalMuted
	// FlagRooted marks a target as rooted.
	FlagRooted
	// FlagSleep marks a target as asleep.
	FlagSleep
	// FlagStunned marks a target as stunned.
	FlagStunned
)

type kind struct {
	typ    Type
	flag   Flag
	debuff bool
}

var coreKinds = map[string]kind{
	"Buff":        {typ: TypeBuff},
	"Debuff":      {typ: TypeDebuff, debuff: true},
	"DamOverTime": {typ: TypeDamOverTime, debuff: true},
	"Fear":        {typ: TypeFear, flag: FlagFear, debuff: true},
	"Root":        {typ: TypeRoot, flag: FlagRooted, debuff: true},
	"Sleep":       {typ: TypeSleep, flag: FlagSleep, debuff: true},
	"Stun":        {typ: TypeStun, flag: FlagStunned, debuff: true},
}

// New builds a runtime effect from a parsed core effect template.
func New(skill Skill, tmpl modelskill.EffectTemplate) (*Effect, error) {
	k, ok := coreKinds[tmpl.Name]
	if !ok {
		return nil, fmt.Errorf("effect: unsupported core effect %q", tmpl.Name)
	}
	if tmpl.AttachCondition != nil {
		return nil, fmt.Errorf("effect %s: attach conditions are not wired yet", tmpl.Name)
	}
	if k.flag == 0 {
		k.flag = FlagNone
	}

	skill.Debuff = skill.Debuff || k.debuff
	e := &Effect{
		Skill:    skill,
		Template: tmpl,
		Type:     k.typ,
		Flag:     k.flag,
	}

	funcs, err := statFuncs(e, tmpl.Funcs)
	if err != nil {
		return nil, fmt.Errorf("effect %s: %w", tmpl.Name, err)
	}
	e.Funcs = funcs
	return e, nil
}

func statFuncs(owner *Effect, templates []modelskill.FuncTemplate) ([]basefunc.Func, error) {
	funcs := make([]basefunc.Func, 0, len(templates))
	for _, tmpl := range templates {
		if tmpl.AttachCondition != nil || tmpl.Condition != nil {
			return nil, fmt.Errorf("conditional stat funcs are not wired yet")
		}
		s, err := stat.ByName(tmpl.Stat)
		if err != nil {
			return nil, err
		}
		fn, err := statFunc(owner, s, tmpl)
		if err != nil {
			return nil, err
		}
		funcs = append(funcs, fn)
	}
	return funcs, nil
}

func statFunc(owner *Effect, s stat.Stat, tmpl modelskill.FuncTemplate) (basefunc.Func, error) {
	switch tmpl.Op {
	case modelskill.FuncAdd:
		return basefunc.NewAdd(owner, s, tmpl.Value, nil), nil
	case modelskill.FuncAddMul:
		return basefunc.NewAddMul(owner, s, tmpl.Value, nil), nil
	case modelskill.FuncSub:
		return basefunc.NewSub(owner, s, tmpl.Value, nil), nil
	case modelskill.FuncSubDiv:
		return basefunc.NewSubDiv(owner, s, tmpl.Value, nil), nil
	case modelskill.FuncMul:
		return basefunc.NewMul(owner, s, tmpl.Value, nil), nil
	case modelskill.FuncBaseMul:
		return basefunc.NewBaseMul(owner, s, tmpl.Value, nil), nil
	case modelskill.FuncDiv:
		return basefunc.NewDiv(owner, s, tmpl.Value, nil), nil
	case modelskill.FuncSet:
		return basefunc.NewSet(owner, s, tmpl.Value, nil), nil
	case modelskill.FuncBaseAdd:
		return basefunc.NewBaseAdd(owner, s, tmpl.Value, nil), nil
	case modelskill.FuncEnchant:
		return nil, fmt.Errorf("enchant stat funcs need an item owner")
	default:
		return nil, fmt.Errorf("unknown stat func op %s", tmpl.Op)
	}
}

// DamageOverTimeInput is the state a periodic HP damage tick needs.
type DamageOverTimeInput struct {
	Dead      bool
	HP        float64
	Damage    float64
	KillByDOT bool
	Toggle    bool
}

// DamageOverTimeResult reports the effect of one periodic HP damage tick.
type DamageOverTimeResult struct {
	Damage           float64
	Continue         bool
	RemovedForLackHP bool
}

// DamageOverTimeTick computes one periodic HP damage tick without mutating
// actor state.
func DamageOverTimeTick(in DamageOverTimeInput) DamageOverTimeResult {
	if in.Dead {
		return DamageOverTimeResult{}
	}

	damage := in.Damage
	if damage >= in.HP {
		if in.Toggle {
			return DamageOverTimeResult{RemovedForLackHP: true}
		}
		if !in.KillByDOT {
			if in.HP <= 1 {
				return DamageOverTimeResult{Continue: true}
			}
			damage = in.HP - 1
		}
	}
	return DamageOverTimeResult{Damage: damage, Continue: true}
}
