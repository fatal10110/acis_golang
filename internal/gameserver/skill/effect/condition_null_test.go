package effect

import (
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// TestFuncConditionNullIsUnconditional: a func's own condition and its
// block's attach <cond> that hold no condition reach FuncTemplate as null,
// which gates nothing (reference FuncTemplate.getFunc and Func.calc test a
// condition only when it is non-null). <game chance="50"/> holds no
// condition; <player bogus="1" level="1"/> is a level-1 check.
func TestFuncConditionNullIsUnconditional(t *testing.T) {
	gameChance := modelskill.Condition{Kind: "game", Attrs: map[string]string{"chance": "50"}}
	bogusLevel1 := modelskill.Condition{Kind: "player", Attrs: map[string]string{"bogus": "1", "level": "1"}}

	for name, tc := range map[string]struct {
		direct *modelskill.Condition
		attach *modelskill.ConditionClause
	}{
		"direct":          {direct: &gameChance},
		"attach":          {attach: &modelskill.ConditionClause{Root: gameChance, MessageID: 113}},
		"both":            {direct: &gameChance, attach: &modelskill.ConditionClause{Root: gameChance}},
		"empty attach":    {attach: &modelskill.ConditionClause{}},
		"unknown element": {direct: &modelskill.Condition{Kind: "targetplayable"}},
	} {
		if cond, err := funcCondition(tc.direct, tc.attach); err != nil || cond != nil {
			t.Errorf("%s: got (%v, %v), want no gate", name, cond, err)
		}
	}

	// An <or> left with no child is still a condition: it never holds.
	orOfNull := modelskill.Condition{Kind: "or", Children: []modelskill.Condition{gameChance}}
	if cond, err := funcCondition(&orOfNull, nil); err != nil || cond == nil || cond.Test(fakeConditionActor{level: 80}) {
		t.Errorf("or of null: got (%v, %v), want a gate that never holds", cond, err)
	}

	// A null attach leaves the func's own condition as the only gate.
	cond, err := funcCondition(&bogusLevel1, &modelskill.ConditionClause{Root: gameChance})
	if err != nil || cond == nil {
		t.Fatalf("funcCondition: (%v, %v), want the level gate", cond, err)
	}
	if !cond.Test(fakeConditionActor{level: 1}) || cond.Test(fakeConditionActor{level: 0}) {
		t.Error("<player bogus level=1> gate should hold at level 1 and fail at level 0")
	}
}

// TestFuncConditionNotOfNullNeverHolds: a <not> around a null condition
// throws in the reference when tested, so the func or effect it gates is
// never produced; the gate never holds.
func TestFuncConditionNotOfNullNeverHolds(t *testing.T) {
	notNull := modelskill.Condition{Kind: "not", Children: []modelskill.Condition{{Kind: "game", Attrs: map[string]string{"chance": "50"}}}}
	cond, err := funcCondition(&notNull, nil)
	if err != nil || cond == nil {
		t.Fatalf("funcCondition: (%v, %v), want a gate", cond, err)
	}
	if cond.Test(fakeConditionActor{level: 80}) {
		t.Error("a <not> around no condition should never hold")
	}
}
