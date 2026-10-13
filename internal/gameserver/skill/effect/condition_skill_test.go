package effect

import (
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/conditions"
)

// TestFuncConditionSkillContext covers #3669: a func's own <skill stat> is
// tested with the skill the stat calculation runs for (Func.calc), its
// block's attached one with the skill owning the func (FuncTemplate.getFunc).
func TestFuncConditionSkillContext(t *testing.T) {
	t.Parallel()
	pAtk := modelskill.Condition{Kind: "skill", Attrs: map[string]string{"stat": "pAtk"}}
	actor := fakeConditionActor{level: 80}
	powerAttack, maxHP := conditions.SkillOf("POWER_ATTACK"), conditions.SkillOf("MAX_HP")

	own, err := funcCondition(&pAtk, nil, maxHP)
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
		if got := own.Test(actor, tc.skill); got != tc.want {
			t.Errorf("own <skill stat=pAtk>, %s: %v, want %v", name, got, tc.want)
		}
	}

	attach := &modelskill.ConditionClause{Root: pAtk}
	for name, tc := range map[string]struct {
		owner conditions.Skill
		want  bool
	}{
		"owned by a POWER_ATTACK skill": {powerAttack, true},
		"owned by a MAX_HP skill":       {maxHP, false},
		"owned by no skill (an item)":   {nil, false},
	} {
		gate, err := funcCondition(nil, attach, tc.owner)
		if err != nil || gate == nil {
			t.Fatalf("attached gate %s: (%v, %v)", name, gate, err)
		}
		for _, calc := range []conditions.Skill{nil, powerAttack, maxHP} {
			if got := gate.Test(actor, calc); got != tc.want {
				t.Errorf("attached <skill stat=pAtk> %s, calculation skill %v: %v, want %v", name, calc, got, tc.want)
			}
		}
	}
}
