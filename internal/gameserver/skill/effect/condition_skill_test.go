package effect

import (
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/conditions"
)

// TestFuncConditionSkillContext covers #3669: a func's own <skill stat> is
// tested with the skill the stat calculation runs for (Func.calc), its
// block's attached one with the skill owning the func when the func is
// attached (FuncTemplate.getFunc).
func TestFuncConditionSkillContext(t *testing.T) {
	t.Parallel()
	pAtk := modelskill.Condition{Kind: "skill", Attrs: map[string]string{"stat": "pAtk"}}
	actor := fakeConditionActor{level: 80}
	powerAttack, maxHP := conditions.SkillOf("POWER_ATTACK"), conditions.SkillOf("MAX_HP")

	own, err := funcCondition(&pAtk)
	if err != nil || own == nil {
		t.Fatalf("own gate: (%v, %v)", own, err)
	}
	for name, tc := range map[string]struct {
		skill conditions.Skill
		want  bool
	}{
		"calculation for a POWER_ATTACK skill": {powerAttack, true},
		"calculation for a MAX_HP skill":       {maxHP, false},
		"calculation for no skill":             {nil, false},
	} {
		if got := own.Test(actor, nil, tc.skill); got != tc.want {
			t.Errorf("own <skill stat=pAtk>, %s: %v, want %v", name, got, tc.want)
		}
	}

	attached := []modelskill.FuncTemplate{{Op: modelskill.FuncAdd, Stat: "pAtk", Value: 1, AttachCondition: &modelskill.ConditionClause{Root: pAtk}}}
	for name, tc := range map[string]struct {
		owner conditions.Skill
		want  bool
	}{
		"owned by a POWER_ATTACK skill": {powerAttack, true},
		"owned by a MAX_HP skill":       {maxHP, false},
		"owned by no skill (an item)":   {nil, false},
	} {
		mods, err := statFuncs(ModOwnerSkill(modelskill.Ref{ID: 1, Level: 1}), attached, tc.owner)
		if err != nil || len(mods) != 1 || mods[0].Cond != nil {
			t.Fatalf("attached func %s: (%v, %v), want one ungated mod", name, mods, err)
		}
		if got := len(Attachable(mods, actor)) == 1; got != tc.want {
			t.Errorf("attached <skill stat=pAtk> %s: attaches %v, want %v", name, got, tc.want)
		}
	}
}
