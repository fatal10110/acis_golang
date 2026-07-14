package effect

import (
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

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

	wantOwner := modelskill.Ref{ID: 134, Level: 1}
	for i, fn := range funcs {
		if fn.Owner() != wantOwner {
			t.Fatalf("funcs[%d].Owner() = %v, want %v", i, fn.Owner(), wantOwner)
		}
	}
	if funcs[0].Stat() != stat.RootVuln {
		t.Fatalf("funcs[0].Stat() = %s, want %s", funcs[0].Stat(), stat.RootVuln)
	}
	if got := funcs[0].Calc(nil, nil, nil, 100, 100); got != 80 {
		t.Fatalf("funcs[0].Calc() = %v, want 80", got)
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
