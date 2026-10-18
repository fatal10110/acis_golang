package effect

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// TestAttachableTestsTheAttachingPair covers #3862: a func's attached
// <cond> is tested when the func is attached (FuncTemplate.getFunc), a
// passive skill's against no target (L2Skill.getStatFuncs passes null), an
// item's against its wearer (Item.getStatFuncs passes the player twice). A
// target-side <target hp_min_max> therefore fails for the skill and holds
// for the item; the func it rejects is left out, the others kept.
func TestAttachableTestsTheAttachingPair(t *testing.T) {
	t.Parallel()
	targetSide := &modelskill.ConditionClause{Root: modelskill.Condition{Kind: "target", Attrs: map[string]string{"hp_min_max": "0,100"}}}
	templates := []modelskill.FuncTemplate{
		{Op: modelskill.FuncAdd, Stat: "pAtk", Value: 1},
		{Op: modelskill.FuncAdd, Stat: "pDef", Value: 2, AttachCondition: targetSide},
	}
	owner := fakeConditionActor{level: 80}
	for name, tc := range map[string]struct {
		owner ModOwner
		want  int
	}{
		"passive skill": {ModOwnerSkill(modelskill.Ref{ID: 1, Level: 1}), 1},
		"item":          {ModOwnerItem(ItemOwner{Tmpl: &item.Template{ID: 1}}), 2},
	} {
		mods, err := statFuncs(tc.owner, templates, nil)
		if err != nil {
			t.Fatalf("%s: statFuncs: %v", name, err)
		}
		kept := Attachable(mods, owner)
		if len(kept) != tc.want || kept[0].Stat != mods[0].Stat {
			t.Errorf("%s: attached %d funcs (%v), want %d", name, len(kept), kept, tc.want)
		}
		if got := Attachable(mods, nil); len(got) != 1 {
			t.Errorf("%s: with no owner view attached %d funcs, want only the unconditioned one", name, len(got))
		}
	}
}
