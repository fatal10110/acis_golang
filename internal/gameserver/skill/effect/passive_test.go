package effect

import (
	"strings"
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// ---- from passive_test.go ----
// The skill id, level, and Funcs below reproduce the "Toughness" passive
// (skill 134): a flat 20% vulnerability increase to three abnormal
// resistances, carried as top-level Funcs rather than an effect template.

func TestPassiveFuncsBuildsFuncsOwnedByTheSkillLevel(t *testing.T) {
	def := modelskill.Definition{
		ID:         134,
		Level:      1,
		Activation: modelskill.ActivationPassive,
		Funcs: []modelskill.FuncTemplate{
			{Op: modelskill.FuncAddMul, Stat: "rootVuln", Value: 20},
			{Op: modelskill.FuncAddMul, Stat: "sleepVuln", Value: 20},
			{Op: modelskill.FuncAddMul, Stat: "poisonVuln", Value: 20},
		},
	}

	funcs, err := PassiveFuncs(def)
	if err != nil {
		t.Fatalf("PassiveFuncs() error: %v", err)
	}
	if len(funcs) != 3 {
		t.Fatalf("Funcs length = %d, want 3", len(funcs))
	}

	wantOwner := ModOwnerSkill(modelskill.Ref{ID: 134, Level: 1})
	for i, fn := range funcs {
		if fn.Owner != wantOwner {
			t.Fatalf("funcs[%d].Owner = %v, want %v", i, fn.Owner, wantOwner)
		}
	}
	if funcs[0].Stat != stat.RootVuln {
		t.Fatalf("funcs[0].Stat = %s, want %s", funcs[0].Stat, stat.RootVuln)
	}
	if got := apply(funcs[0], nil, 100, 100); got != 80 {
		t.Fatalf("apply(funcs[0]) = %v, want 80", got)
	}
}

func TestPassiveFuncsRejectsNonPassiveSkill(t *testing.T) {
	def := modelskill.Definition{ID: 60, Level: 1, Activation: modelskill.ActivationToggle}

	if _, err := PassiveFuncs(def); err == nil {
		t.Fatal("PassiveFuncs() error = nil, want an error for a non-passive skill")
	}
}

func TestPassiveFuncsPropagatesBuildErrors(t *testing.T) {
	def := modelskill.Definition{
		ID:         1,
		Level:      1,
		Activation: modelskill.ActivationPassive,
		Funcs:      []modelskill.FuncTemplate{{Op: modelskill.FuncEnchant, Stat: "pAtk", Value: 1}},
	}

	if _, err := PassiveFuncs(def); err == nil {
		t.Fatal("PassiveFuncs() error = nil, want an error for an ownerless enchant func")
	}
}

func TestSkillStatFuncsIgnoresOperateType(t *testing.T) {
	def := modelskill.Definition{
		ID:         4408,
		Level:      1,
		Activation: modelskill.ActivationActive,
		Funcs:      []modelskill.FuncTemplate{{Op: modelskill.FuncAdd, Stat: "maxHp", Value: 250}},
	}

	if _, err := PassiveFuncs(def); err == nil {
		t.Fatal("PassiveFuncs() error = nil, want an error for an active-operate skill")
	}

	funcs, err := SkillStatFuncs(def)
	if err != nil {
		t.Fatalf("SkillStatFuncs() error: %v", err)
	}
	if len(funcs) != 1 {
		t.Fatalf("SkillStatFuncs() length = %d, want 1", len(funcs))
	}
	if funcs[0].Owner != ModOwnerSkill(modelskill.Ref{ID: 4408, Level: 1}) {
		t.Fatalf("Owner = %v, want skill 4408 level 1", funcs[0].Owner)
	}
	if funcs[0].Stat != stat.MaxHP || funcs[0].Op != OpAdd || funcs[0].Value != 250 {
		t.Fatalf("func = %+v, want maxHp add 250", funcs[0])
	}
}

func TestTemplatePassiveModsResolvesRefsAndSkipsMissing(t *testing.T) {
	table := modelskill.NewTable([]modelskill.Definition{{
		ID:         99,
		Level:      1,
		Activation: modelskill.ActivationActive,
		Funcs:      []modelskill.FuncTemplate{{Op: modelskill.FuncAdd, Stat: "maxHp", Value: 250}},
	}})

	got, err := TemplatePassiveMods(table, []modelskill.Ref{
		{ID: 99, Level: 1},
		{ID: 98, Level: 1},
	})
	if err != nil {
		t.Fatalf("TemplatePassiveMods() error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("TemplatePassiveMods() length = %d, want 1 (missing ref skipped)", len(got))
	}
	if got[0].Value != 250 {
		t.Fatalf("Value = %v, want 250", got[0].Value)
	}

	fns, err := TemplatePassiveMods(nil, []modelskill.Ref{{ID: 99, Level: 1}})
	if err != nil {
		t.Fatalf("nil lookup error: %v", err)
	}
	if len(fns) != 0 {
		t.Fatalf("nil lookup = %v, want empty", fns)
	}
}

func TestTemplatePassiveModsSkipsEnchantAndReportsBuildErrors(t *testing.T) {
	table := modelskill.NewTable([]modelskill.Definition{
		{
			ID:    1,
			Level: 1,
			Funcs: []modelskill.FuncTemplate{{Op: modelskill.FuncEnchant, Stat: "pAtk", Value: 1}},
		},
		{
			ID:    2,
			Level: 1,
			Funcs: []modelskill.FuncTemplate{{Op: modelskill.FuncAdd, Stat: "notAStat", Value: 1}},
		},
		{
			ID:    3,
			Level: 1,
			Funcs: []modelskill.FuncTemplate{{Op: modelskill.FuncAdd, Stat: "maxHp", Value: 250}},
		},
	})

	got, err := TemplatePassiveMods(table, []modelskill.Ref{{ID: 1, Level: 1}, {ID: 3, Level: 1}})
	if err != nil {
		t.Fatalf("enchant skip error: %v", err)
	}
	if len(got) != 1 || got[0].Value != 250 {
		t.Fatalf("enchant skip = %+v, want maxHp add 250", got)
	}

	_, err = TemplatePassiveMods(table, []modelskill.Ref{{ID: 2, Level: 1}})
	if err == nil {
		t.Fatal("TemplatePassiveMods() error = nil, want unknown-stat build error")
	}
	if !strings.Contains(err.Error(), "template passive skill 2 level 1") {
		t.Fatalf("error = %v, want skill 2 level 1 wrapped", err)
	}
}
