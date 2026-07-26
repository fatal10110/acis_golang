package effect

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/basefunc"
)

func TestItemModifierFuncsBuildsAddSetAndEnchantFuncs(t *testing.T) {
	tmpl := &item.Template{
		ID:      100,
		Crystal: item.CrystalS,
		Weapon:  &item.WeaponDetail{Type: item.WeaponSword},
		Modifiers: []item.StatModifier{
			{Op: item.FuncAdd, Stat: "pAtk", Value: 10},
			{Op: item.FuncSet, Stat: "pAtkSpd", Value: 300},
			{Op: item.FuncEnchant, Stat: "pAtk", Value: 0},
		},
	}
	inst := &item.Instance{ObjectID: 1, TemplateID: 100, EnchantLevel: 4}
	owner := ItemOwner{Inst: inst, Tmpl: tmpl}

	fns, err := ItemModifierFuncs(owner)
	if err != nil {
		t.Fatalf("ItemModifierFuncs() error: %v", err)
	}
	if len(fns) != 3 {
		t.Fatalf("len(fns) = %d, want 3", len(fns))
	}
	if _, ok := fns[2].(*basefunc.Enchant); !ok {
		t.Fatalf("fns[2] = %T, want *basefunc.Enchant", fns[2])
	}
	for _, fn := range fns {
		if fn.Owner() != owner {
			t.Fatalf("Owner() = %v, want %v", fn.Owner(), owner)
		}
	}
}

func TestItemModifierFuncsRejectsConditionalModifier(t *testing.T) {
	tmpl := &item.Template{
		ID: 101,
		Modifiers: []item.StatModifier{
			{Op: item.FuncAdd, Stat: "pAtk", Value: 10, Condition: &item.Condition{}},
		},
	}
	owner := ItemOwner{Inst: &item.Instance{ObjectID: 1, TemplateID: 101}, Tmpl: tmpl}

	if _, err := ItemModifierFuncs(owner); err == nil {
		t.Fatal("ItemModifierFuncs() error = nil, want error for a conditional modifier")
	}
}

func TestItemOwnerEnchantLevelReadsLiveInstanceState(t *testing.T) {
	tmpl := &item.Template{ID: 102, Crystal: item.CrystalD}
	inst := &item.Instance{ObjectID: 1, TemplateID: 102, EnchantLevel: 0}
	owner := ItemOwner{Inst: inst, Tmpl: tmpl}

	if got := owner.EnchantLevel(); got != 0 {
		t.Fatalf("EnchantLevel() = %d, want 0", got)
	}
	inst.SetEnchantLevel(5)
	if got := owner.EnchantLevel(); got != 5 {
		t.Fatalf("EnchantLevel() after SetEnchantLevel(5) = %d, want 5 (must read live state, not a captured value)", got)
	}
}

func TestItemPassiveFuncsOnlyAppliesLoadedPassiveSkills(t *testing.T) {
	skills := modelskill.NewTable([]modelskill.Definition{
		{ID: 200, Level: 1, Activation: modelskill.ActivationPassive, Funcs: []modelskill.FuncTemplate{
			{Op: modelskill.FuncAdd, Stat: "pAtk", Value: 12},
		}},
		{ID: 201, Level: 1, Activation: modelskill.ActivationToggle},
	})
	tmpl := &item.Template{
		ID: 103,
		AttachedSkills: []item.SkillRef{
			{ID: 200, Level: 1}, // passive: contributes
			{ID: 201, Level: 1}, // not passive: skipped
			{ID: 999, Level: 1}, // unloaded: skipped
		},
	}
	owner := ItemOwner{Inst: &item.Instance{ObjectID: 1, TemplateID: 103}, Tmpl: tmpl}

	fns, err := ItemPassiveFuncs(skills, owner)
	if err != nil {
		t.Fatalf("ItemPassiveFuncs() error: %v", err)
	}
	if len(fns) != 1 {
		t.Fatalf("len(fns) = %d, want 1", len(fns))
	}
	if fns[0].Owner() != owner {
		t.Fatalf("Owner() = %v, want %v", fns[0].Owner(), owner)
	}
}
