package effect

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/basefunc"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// ItemOwner is the owner and basefunc.EnchantedItem source for stat
// functions one equipped item instance contributes (its AttachedSkills
// passives and Modifiers). Equality between two ItemOwner values compares
// Inst and Tmpl, so RemoveStatsByOwner drops only the functions attached for
// that specific instance even when another equipped item shares its
// template. EnchantLevel reads Inst's live snapshot on every call rather
// than a value captured at attach time, so a scroll of enchant used on an
// already-equipped item needs no explicit re-attach.
type ItemOwner struct {
	Inst *item.Instance
	Tmpl *item.Template
}

// EnchantLevel satisfies basefunc.EnchantedItem.
func (o ItemOwner) EnchantLevel() int {
	if o.Inst == nil {
		return 0
	}
	return o.Inst.Snapshot().EnchantLevel
}

// Weapon satisfies basefunc.EnchantedItem.
func (o ItemOwner) Weapon() (item.WeaponType, bool) {
	if o.Tmpl == nil || o.Tmpl.Weapon == nil {
		return 0, false
	}
	return o.Tmpl.Weapon.Type, true
}

// Crystal satisfies basefunc.EnchantedItem.
func (o ItemOwner) Crystal() item.CrystalType {
	if o.Tmpl == nil {
		return item.CrystalNone
	}
	return o.Tmpl.Crystal
}

// ItemModifierFuncs builds the stat functions owner.Tmpl.Modifiers
// contributes while equipped, attributed to owner.
func ItemModifierFuncs(owner ItemOwner) ([]basefunc.Func, error) {
	if owner.Tmpl == nil {
		return nil, nil
	}
	funcs := make([]basefunc.Func, 0, len(owner.Tmpl.Modifiers))
	for _, mod := range owner.Tmpl.Modifiers {
		if mod.AttachCondition != nil || mod.Condition != nil {
			return nil, fmt.Errorf("item %d: conditional stat modifiers are not wired yet", owner.Tmpl.ID)
		}
		s, err := stat.ByName(mod.Stat)
		if err != nil {
			return nil, fmt.Errorf("item %d: %w", owner.Tmpl.ID, err)
		}
		fn, err := itemStatFunc(owner, s, mod)
		if err != nil {
			return nil, fmt.Errorf("item %d: %w", owner.Tmpl.ID, err)
		}
		funcs = append(funcs, fn)
	}
	return funcs, nil
}

func itemStatFunc(owner ItemOwner, s stat.Stat, mod item.StatModifier) (basefunc.Func, error) {
	switch mod.Op {
	case item.FuncAdd:
		return basefunc.NewAdd(owner, s, mod.Value, nil), nil
	case item.FuncAddMul:
		return basefunc.NewAddMul(owner, s, mod.Value, nil), nil
	case item.FuncSub:
		return basefunc.NewSub(owner, s, mod.Value, nil), nil
	case item.FuncSubDiv:
		return basefunc.NewSubDiv(owner, s, mod.Value, nil), nil
	case item.FuncMul:
		return basefunc.NewMul(owner, s, mod.Value, nil), nil
	case item.FuncBaseMul:
		return basefunc.NewBaseMul(owner, s, mod.Value, nil), nil
	case item.FuncDiv:
		return basefunc.NewDiv(owner, s, mod.Value, nil), nil
	case item.FuncSet:
		return basefunc.NewSet(owner, s, mod.Value, nil), nil
	case item.FuncBaseAdd:
		return basefunc.NewBaseAdd(owner, s, mod.Value, nil), nil
	case item.FuncEnchant:
		return basefunc.NewEnchant(owner, s, mod.Value, nil), nil
	default:
		return nil, fmt.Errorf("unknown item stat modifier op %s", mod.Op)
	}
}

// ItemPassiveFuncs builds the stat functions owner.Tmpl.AttachedSkills
// contributes while equipped: only ids resolving to a loaded passive skill
// definition contribute, mirroring persistence.SetKnownSkill's
// learned-passive path. A missing or non-passive entry is silently
// skipped — a template may name an active-use item skill in the same list.
func ItemPassiveFuncs(skills *modelskill.Table, owner ItemOwner) ([]basefunc.Func, error) {
	if owner.Tmpl == nil || skills == nil {
		return nil, nil
	}
	var funcs []basefunc.Func
	for _, ref := range owner.Tmpl.AttachedSkills {
		def, ok := skills.Get(modelskill.ID(ref.ID), int(ref.Level))
		if !ok || def.Activation != modelskill.ActivationPassive {
			continue
		}
		fns, err := statFuncs(owner, def.Funcs)
		if err != nil {
			return nil, fmt.Errorf("item %d passive skill %d level %d: %w", owner.Tmpl.ID, ref.ID, ref.Level, err)
		}
		funcs = append(funcs, fns...)
	}
	return funcs, nil
}
