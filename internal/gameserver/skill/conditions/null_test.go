package conditions

import (
	"fmt"
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// The two null-condition fixtures: the reference condition reader
// (DocumentBase.parseCondition and its parse*Condition methods) returns null
// for <game chance="50"/>, which has no recognized attribute, and reads
// <player bogus="1" level="1"/> as ConditionPlayerLevel(1), ignoring the
// unrecognized attribute.
var (
	gameChance     = leaf("game", map[string]string{"chance": "50"})
	bogusLevel1    = leaf("player", map[string]string{"bogus": "1", "level": "1"})
	playerLevel    = func(n string) modelskill.Condition { return leaf("player", map[string]string{"level": n}) }
	nullConditions = []struct {
		name string
		node modelskill.Condition
	}{
		{"game chance", gameChance},
		{"no attribute player", leaf("player", nil)},
		{"empty target", leaf("target", nil)},
		{"unknown target attribute", leaf("target", map[string]string{"bogus": "1"})},
		{"unknown using attribute", leaf("using", map[string]string{"bogus": "1"})},
		{"player with only zero seeds", leaf("player", map[string]string{"seed_fire": "0"})},
		{"player with forces summing to zero", leaf("player", map[string]string{"battle_force": "-1", "spell_force": "1"})},
		{"unknown element", leaf("bogus", map[string]string{"level": "1"})},
		{"zero condition", modelskill.Condition{}},
		{"not with no child", leaf("not", nil)},
	}
)

func TestCompileNullConditions(t *testing.T) {
	for _, tc := range nullConditions {
		c, err := Compile(tc.node)
		if c != nil || err != nil {
			t.Errorf("%s: compiled to (%#v, %v), want no condition", tc.name, c, err)
		}
		if !IsNull(tc.node) {
			t.Errorf("%s: IsNull = false, want true", tc.name)
		}
	}
	for _, node := range []modelskill.Condition{
		bogusLevel1,
		leaf("and", nil),
		leaf("or", nil, gameChance),
		leaf("not", nil, gameChance),
		leaf("using", map[string]string{"kind": ""}),
		leaf("player", map[string]string{"level": "x"}),
		leaf("skill", map[string]string{"stat": "pAtk"}),
	} {
		if IsNull(node) {
			t.Errorf("IsNull(%+v) = true, want false", node)
		}
	}
}

// TestCompileIgnoresUnrecognizedAttributes: an unrecognized attribute next
// to recognized ones is ignored, even one that would not decode.
func TestCompileIgnoresUnrecognizedAttributes(t *testing.T) {
	cases := []struct {
		node modelskill.Condition
		want Condition
	}{
		{bogusLevel1, Level{Level: 1}},
		{leaf("player", map[string]string{"bogus": "x", "level": "1"}), Level{Level: 1}},
		{leaf("target", map[string]string{"bogus": "1", "npcId": "5"}), TargetNpcID{IDs: []int{5}}},
		{leaf("game", map[string]string{"chance": "50", "night": "true"}), GameTime{Night: true}},
		{leaf("using", map[string]string{"bogus": "1", "kind": ""}), UsingItemType{}},
	}
	for _, tc := range cases {
		c := mustCompile(t, tc.node)
		if fmt.Sprintf("%#v", c) != fmt.Sprintf("%#v", tc.want) {
			t.Errorf("%+v compiled %#v, want %#v", tc.node, c, tc.want)
		}
	}
}

// TestLogicDropsNullChildren: <and>/<or> drop a child that holds no
// condition (ConditionLogicAnd/Or.add return on null), so an <or> left with
// none never holds and an <and> left with none always does.
func TestLogicDropsNullChildren(t *testing.T) {
	p40 := &player{creature: creature{level: 40}}
	runProbes(t, leaf("and", nil, gameChance, playerLevel("50")), []probe{{"and drops null, level fails", p40, nil, false}})
	runProbes(t, leaf("and", nil, gameChance, playerLevel("30")), []probe{{"and drops null, level holds", p40, nil, true}})
	runProbes(t, leaf("and", nil, gameChance), []probe{{"and of nulls", p40, nil, true}})
	runProbes(t, leaf("or", nil, gameChance), []probe{{"or of nulls", p40, nil, false}})
	runProbes(t, leaf("or", nil, gameChance, bogusLevel1), []probe{{"or keeps level 1", p40, nil, true}})
}

// TestNotReadsFirstChildOnly: parseLogicNot wraps the first element child
// and never reads the rest (probe: <not><player level="1"/><player
// hp="#t"/></not> loads as ConditionLogicNot(ConditionPlayerLevel(1))).
func TestNotReadsFirstChildOnly(t *testing.T) {
	node := leaf("not", nil, playerLevel("1"), leaf("player", map[string]string{"hp": "#t"}), leaf("player", map[string]string{"level": "x"}))
	c := mustCompile(t, node)
	if c != (Not{Condition: Level{Level: 1}}) {
		t.Fatalf("compiled %#v, want Not{Level 1}", c)
	}
	runProbes(t, node, []probe{
		{"level 40", &player{creature: creature{level: 40}}, nil, false},
		{"level 0", &player{}, nil, true},
	})
	runProbes(t, leaf("not", nil, bogusLevel1), []probe{{"not level 1 at 40", &player{creature: creature{level: 40}}, nil, false}})
}

// TestEvaluateAbortsOnNotOfNull: a <not> around a null child tests as a
// NullPointerException in the reference. Evaluate reports that as an abort,
// but only when the test reaches it: <and>/<or> stop at the first child that
// decides the result.
func TestEvaluateAbortsOnNotOfNull(t *testing.T) {
	p40 := &player{creature: creature{level: 40}}
	notNull := leaf("not", nil, gameChance)
	cases := []struct {
		name          string
		node          modelskill.Condition
		held, aborted bool
	}{
		{"not of null", notNull, false, true},
		{"and fails before it", leaf("and", nil, playerLevel("50"), notNull), false, false},
		{"and reaches it", leaf("and", nil, playerLevel("30"), notNull), false, true},
		{"or holds before it", leaf("or", nil, playerLevel("30"), notNull), true, false},
		{"or reaches it", leaf("or", nil, playerLevel("50"), notNull), false, true},
		{"not of not of null", leaf("not", nil, notNull), false, true},
	}
	for _, tc := range cases {
		held, aborted := Evaluate(mustCompile(t, tc.node), p40, nil, nil)
		if held != tc.held || aborted != tc.aborted {
			t.Errorf("%s: Evaluate = (%v, %v), want (%v, %v)", tc.name, held, aborted, tc.held, tc.aborted)
		}
		if got := mustCompile(t, tc.node).Test(p40, nil, nil); got != tc.held {
			t.Errorf("%s: Test = %v, want %v", tc.name, got, tc.held)
		}
	}
	if held, aborted := Evaluate(nil, p40, nil, nil); held || !aborted {
		t.Errorf("Evaluate(nil) = (%v, %v), want (false, true)", held, aborted)
	}
}

// TestEvaluateSkillNullClause: a skill <cond> that holds no condition is
// attached as null (L2Skill.attach), and checkCondition throws on it, so the
// cast fails before any feedback is sent; a <not> around a null condition
// throws when the test reaches it. A clause that holds a condition refuses
// with its own feedback.
func TestEvaluateSkillNullClause(t *testing.T) {
	caster := source{&player{creature: creature{level: 40}}}
	refuse := func(root modelskill.Condition) (modelskill.ConditionClause, bool) {
		def := modelskill.Definition{Conditions: []modelskill.ConditionClause{
			{Root: root, MessageID: 113, AddName: true, Message: "m"},
		}}
		return EvaluateSkill(def, caster, nil)
	}

	for _, root := range []modelskill.Condition{gameChance, {}, leaf("not", nil, gameChance), leaf("and", nil, playerLevel("1"), leaf("not", nil, gameChance))} {
		failed, ok := refuse(root)
		if ok {
			t.Errorf("%+v: cast allowed, want refused", root)
			continue
		}
		if failed.MessageID != 0 || failed.AddName || failed.Message != "" {
			t.Errorf("%+v: refused with feedback %+v, want none", root, failed)
		}
	}

	if _, ok := refuse(bogusLevel1); !ok {
		t.Error("<player bogus level=1> at level 40: refused, want allowed")
	}
	if failed, ok := refuse(leaf("not", nil, bogusLevel1)); ok || failed.MessageID != 113 {
		t.Errorf("<not><player bogus level=1/></not> at level 40: got (%+v, %v), want refused with its message", failed, ok)
	}
	if failed, ok := refuse(leaf("and", nil, playerLevel("50"), leaf("not", nil, gameChance))); ok || failed.MessageID != 113 {
		t.Errorf("and failing before the null not: got (%+v, %v), want refused with its message", failed, ok)
	}

	// An earlier clause that fails still answers with its own feedback.
	def := modelskill.Definition{Conditions: []modelskill.ConditionClause{
		{Root: playerLevel("50"), MessageID: 7},
		{Root: gameChance},
	}}
	if failed, ok := EvaluateSkill(def, caster, nil); ok || failed.MessageID != 7 {
		t.Errorf("failing clause before a null one: got (%+v, %v), want the first refused", failed, ok)
	}
}
