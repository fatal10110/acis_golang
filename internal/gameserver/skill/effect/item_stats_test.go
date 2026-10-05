package effect

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// ---- from item_stats_test.go ----
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
	if fns[2].Op != OpEnchant {
		t.Fatalf("fns[2].Op = %v, want OpEnchant", fns[2].Op)
	}
	for _, fn := range fns {
		if fn.Owner != ModOwnerItem(owner) {
			t.Fatalf("Owner = %v, want %v", fn.Owner, ModOwnerItem(owner))
		}
	}
}

func TestItemModifierFuncsRejectsConditionalModifier(t *testing.T) {
	tmpl := &item.Template{
		ID: 101,
		Modifiers: []item.StatModifier{
			{Op: item.FuncAdd, Stat: "pAtk", Value: 10, Condition: &item.Condition{Kind: "skill", Attrs: map[string]string{"stat": "pAtk"}}},
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
