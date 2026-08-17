package effect

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// ItemOwner is the owner and item enchant-data source for stat
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

// EnchantLevel reads Inst's live enchant level for applyEnchant.
func (o ItemOwner) EnchantLevel() int {
	if o.Inst == nil {
		return 0
	}
	return o.Inst.Snapshot().EnchantLevel
}

// Weapon reports Tmpl's weapon type, if any, for applyEnchant.
func (o ItemOwner) Weapon() (item.WeaponType, bool) {
	if o.Tmpl == nil || o.Tmpl.Weapon == nil {
		return 0, false
	}
	return o.Tmpl.Weapon.Type, true
}

// Crystal reports Tmpl's crystal grade for applyEnchant.
func (o ItemOwner) Crystal() item.CrystalType {
	if o.Tmpl == nil {
		return item.CrystalNone
	}
	return o.Tmpl.Crystal
}

// ItemModifierFuncs builds the stat functions owner.Tmpl.Modifiers
// contributes while equipped, attributed to owner.
func ItemModifierFuncs(owner ItemOwner) ([]Mod, error) {
	if owner.Tmpl == nil {
		return nil, nil
	}
	mods := make([]Mod, 0, len(owner.Tmpl.Modifiers))
	for _, mod := range owner.Tmpl.Modifiers {
		gate, err := itemModifierCondition(mod)
		if err != nil {
			return nil, fmt.Errorf("item %d: %w", owner.Tmpl.ID, err)
		}
		s, err := stat.ByName(mod.Stat)
		if err != nil {
			return nil, fmt.Errorf("item %d: %w", owner.Tmpl.ID, err)
		}
		m, err := itemStatFunc(owner, s, mod, gate)
		if err != nil {
			return nil, fmt.Errorf("item %d: %w", owner.Tmpl.ID, err)
		}
		mods = append(mods, m)
	}
	return mods, nil
}

// itemModifierCondition builds the same effector-side gate as
// funcCondition, converting item.StatModifier's own Condition/
// AttachCondition (an XML-shape twin of modelskill.Condition/
// ConditionClause, parsed independently for item templates) to the shared
// tree first.
func itemModifierCondition(mod item.StatModifier) (Condition, error) {
	var direct *modelskill.Condition
	if mod.Condition != nil {
		c := convertItemCondition(*mod.Condition)
		direct = &c
	}
	var attach *modelskill.ConditionClause
	if mod.AttachCondition != nil {
		attach = &modelskill.ConditionClause{
			Root:      convertItemCondition(mod.AttachCondition.Root),
			Message:   mod.AttachCondition.Message,
			MessageID: mod.AttachCondition.MessageID,
			AddName:   mod.AttachCondition.AddName,
		}
	}
	return funcCondition(direct, attach)
}

func convertItemCondition(c item.Condition) modelskill.Condition {
	out := modelskill.Condition{Kind: c.Kind, Attrs: c.Attrs}
	for _, ch := range c.Children {
		out.Children = append(out.Children, convertItemCondition(ch))
	}
	return out
}

func itemStatFunc(owner ItemOwner, s stat.Stat, mod item.StatModifier, cond Condition) (Mod, error) {
	modOwner := ModOwnerItem(owner)
	op, err := itemOp(mod.Op)
	if err != nil {
		return Mod{}, err
	}
	return Mod{Stat: s, Op: op, Value: mod.Value, Cond: cond, Owner: modOwner}, nil
}

func itemOp(op item.FuncOp) (Op, error) {
	switch op {
	case item.FuncAdd:
		return OpAdd, nil
	case item.FuncAddMul:
		return OpAddMul, nil
	case item.FuncSub:
		return OpSub, nil
	case item.FuncSubDiv:
		return OpSubDiv, nil
	case item.FuncMul:
		return OpMul, nil
	case item.FuncBaseMul:
		return OpBaseMul, nil
	case item.FuncDiv:
		return OpDiv, nil
	case item.FuncSet:
		return OpSet, nil
	case item.FuncBaseAdd:
		return OpBaseAdd, nil
	case item.FuncEnchant:
		return OpEnchant, nil
	default:
		return 0, fmt.Errorf("unknown item stat modifier op %s", op)
	}
}

// ItemPassiveFuncs builds the stat functions owner.Tmpl.AttachedSkills
// contributes while equipped: only ids resolving to a loaded passive skill
// definition contribute, mirroring persistence.SetKnownSkill's
// learned-passive path. A missing or non-passive entry is silently
// skipped — a template may name an active-use item skill in the same list.
func ItemPassiveFuncs(skills *modelskill.Table, owner ItemOwner) ([]Mod, error) {
	if owner.Tmpl == nil || skills == nil {
		return nil, nil
	}
	var mods []Mod
	modOwner := ModOwnerItem(owner)
	add := func(ref item.SkillRef, cond Condition) error {
		def, ok := skills.Get(modelskill.ID(ref.ID), int(ref.Level))
		if !ok || def.Activation != modelskill.ActivationPassive {
			return nil
		}
		fns, err := statFuncs(modOwner, def.Funcs, cond)
		if err != nil {
			return fmt.Errorf("item %d passive skill %d level %d: %w", owner.Tmpl.ID, ref.ID, ref.Level, err)
		}
		mods = append(mods, fns...)
		return nil
	}
	for _, ref := range owner.Tmpl.AttachedSkills {
		if err := add(ref, nil); err != nil {
			return nil, err
		}
	}
	if weapon := owner.Tmpl.Weapon; weapon != nil && weapon.Enchant4Skill != nil {
		if err := add(*weapon.Enchant4Skill, enchantAtLeast{owner: owner, level: 4}); err != nil {
			return nil, err
		}
	}
	return mods, nil
}

type enchantAtLeast struct {
	owner ItemOwner
	level int
}

func (c enchantAtLeast) Test(stat.Actor) bool {
	return c.owner.EnchantLevel() >= c.level
}
