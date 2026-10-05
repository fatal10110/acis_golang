package xml

import (
	encxml "encoding/xml"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/conditions"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

// TestSkillCondThatHoldsNoConditionReadsNoFeedback: the reference reads a
// <cond>'s msg/msgId/addName only when its predicate is a condition, so a
// "#t" msgId (which no cond may name) beside <game chance="50"/> loads,
// while the same msgId beside <player bogus="1" level="1"/> (a level check)
// fails the skill. A skill-level <cond> that holds no condition, or has no
// predicate at all, still attaches (L2Skill.attach adds null), and every
// cast then fails with no feedback.
func TestSkillCondThatHoldsNoConditionReadsNoFeedback(t *testing.T) {
	t.Parallel()
	caster := condNullCaster{}
	for name, body := range map[string]string{
		"game chance":  `<cond msgId="#t" addName="1"><game chance="50"/></cond>`,
		"no predicate": `<cond msgId="1"/>`,
		"empty not":    `<cond msgId="#t"><not/></cond>`,
	} {
		table := loadTemplateParitySkill(t, body)
		for _, level := range templateParityLevels {
			def, ok := table.Get(1, level)
			if !ok {
				t.Errorf("%s: level %d not loaded", name, level)
				continue
			}
			if len(def.Conditions) != 1 {
				t.Errorf("%s: level %d conditions = %+v, want one null clause", name, level, def.Conditions)
				continue
			}
			c := def.Conditions[0]
			if c.MessageID != 0 || c.AddName || c.Message != "" || !conditions.IsNull(c.Root) {
				t.Errorf("%s: level %d clause = %+v, want a null root with no feedback", name, level, c)
			}
			if failed, ok := conditions.EvaluateSkill(def, caster, nil); ok || failed.MessageID != 0 {
				t.Errorf("%s: level %d cast = (%+v, %v), want refused with no feedback", name, level, failed, ok)
			}
		}
	}

	if _, ok := loadTemplateParitySkill(t, `<cond msgId="#t"><player bogus="1" level="1"/></cond>`).Get(1, 1); ok {
		t.Error("a table msgId beside a condition: skill loaded, want it rejected")
	}
	def, ok := loadTemplateParitySkill(t, `<cond msgId="113"><player bogus="1" level="1"/></cond>`).Get(1, 1)
	if !ok {
		t.Fatal("<player bogus level=1>: skill not loaded")
	}
	if len(def.Conditions) != 1 || def.Conditions[0].MessageID != 113 {
		t.Fatalf("<player bogus level=1>: conditions = %+v, want one clause with msgId 113", def.Conditions)
	}
	if _, ok := conditions.EvaluateSkill(def, caster, nil); !ok {
		t.Error("<player bogus level=1> at level 40: cast refused, want allowed")
	}
}

// TestSkillForCondThatHoldsNoConditionGatesNothing: a <for> block's leading
// <cond> that holds no condition reads no msgId and gates nothing.
func TestSkillForCondThatHoldsNoConditionGatesNothing(t *testing.T) {
	t.Parallel()
	table := loadTemplateParitySkill(t, `<for><cond msgId="#t"><game chance="50"/></cond><add stat="pAtk" val="1"/></for>`)
	def, ok := table.Get(1, 1)
	if !ok {
		t.Fatal("level 1 not loaded")
	}
	if len(def.Funcs) != 1 {
		t.Fatalf("funcs = %+v, want one", def.Funcs)
	}
	if ac := def.Funcs[0].AttachCondition; ac != nil && (ac.MessageID != 0 || !conditions.IsNull(ac.Root)) {
		t.Errorf("attach condition = %+v, want none or a null root without feedback", ac)
	}
}

// TestItemCondThatHoldsNoCondition: an item <cond> that holds no condition
// attaches null, which Item.checkCondition skips, and reads no msgId (probe:
// <cond msgId="#10"><player/></cond> loads). The same msgId beside a
// condition fails the item; a <for> block's leading one gates nothing.
func TestItemCondThatHoldsNoCondition(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeItemFile(t, dir, "fixture.xml", `
		<item id="70" type="EtcItem" name="a"><cond msgId="#10"><player/></cond></item>
		<item id="71" type="EtcItem" name="a"><cond msgId="#10"><game chance="50"/></cond></item>
		<item id="72" type="EtcItem" name="a"><cond msgId="1"></cond></item>
		<item id="73" type="EtcItem" name="a"><cond msgId="#10"><player level="1"/></cond></item>
		<item id="74" type="EtcItem" name="a"><cond msg="m"><player bogus="1" level="1"/></cond></item>
		<item id="75" type="EtcItem" name="a"><for><cond msgId="#10"><game chance="50"/></cond><add stat="pAtk" val="1"/></for></item>
		<item id="76" type="EtcItem" name="a"><cond msgId="#10"><and><game chance="50"/></and></cond></item>`)

	table, err := LoadItemTemplates(dir, zerolog.Nop())
	if err != nil {
		t.Fatalf("LoadItemTemplates: %v", err)
	}
	for _, id := range []int32{70, 71, 72} {
		tpl, ok := table.Get(id)
		if !ok {
			t.Errorf("item %d not loaded", id)
			continue
		}
		if len(tpl.UseConditions) != 0 {
			t.Errorf("item %d use conditions = %+v, want none", id, tpl.UseConditions)
		}
	}
	for _, id := range []int32{73, 76} {
		if _, ok := table.Get(id); ok {
			t.Errorf("item %d loaded, want a table msgId beside a condition rejected", id)
		}
	}
	if tpl, ok := table.Get(74); !ok || len(tpl.UseConditions) != 1 || tpl.UseConditions[0].Message != "m" {
		t.Errorf("item 74: want one use condition with msg m")
	}
	tpl, ok := table.Get(75)
	if !ok || len(tpl.Modifiers) != 1 || tpl.Modifiers[0].AttachCondition != nil {
		t.Errorf("item 75: want one ungated pAtk func")
	}
}

// condNullCaster is a level-40 player condition view.
type condNullCaster struct{}

func (condNullCaster) ConditionActor() conditions.Actor { return condNullActor{} }

type condNullActor struct{ conditions.Actor }

func (condNullActor) Level() int { return 40 }

// TestShippedConditionsHoldNoNullNode pins that the null-condition rules
// leave shipped skills and items unchanged: no shipped condition node holds
// no condition, no <not> has other than one child, and every attribute of a
// <player>/<target>/<using>/<game> is one the reader recognizes (a leaf with
// that attribute alone is a condition).
func TestShippedConditionsHoldNoNullNode(t *testing.T) {
	t.Parallel()
	skills, err := LoadSkillDefinitions(datapackPath(t, filepath.Join("data", "xml", "skills")), zerolog.Nop())
	if err != nil {
		t.Fatalf("LoadSkillDefinitions: %v", err)
	}
	items, err := LoadItemTemplates(datapackPath(t, filepath.Join("data", "xml", "items")), zerolog.Nop())
	if err != nil {
		t.Fatalf("LoadItemTemplates: %v", err)
	}

	checked := 0
	check := func(where string, node skill.Condition) {
		checked++
		for _, problem := range nullNodes(node) {
			t.Errorf("%s: %s", where, problem)
		}
	}
	funcs := func(where string, fns []skill.FuncTemplate) {
		for _, fn := range fns {
			if fn.Condition != nil {
				check(where+" func "+fn.Stat, *fn.Condition)
			}
			if fn.AttachCondition != nil {
				check(where+" func "+fn.Stat+" attach", fn.AttachCondition.Root)
			}
		}
	}
	for _, def := range skills.All() {
		where := fmt.Sprintf("skill %d level %d", def.ID, def.Level)
		for _, c := range def.Conditions {
			check(where+" cond", c.Root)
		}
		funcs(where, def.Funcs)
		for _, eff := range append(append([]skill.EffectTemplate{}, def.Effects...), def.SelfEffects...) {
			if eff.AttachCondition != nil {
				check(where+" effect "+eff.Name+" attach", eff.AttachCondition.Root)
			}
			funcs(where+" effect "+eff.Name, eff.Funcs)
		}
	}
	for _, tpl := range items.All() {
		where := fmt.Sprintf("item %d", tpl.ID)
		for _, uc := range tpl.UseConditions {
			check(where+" cond", effect.ItemCondition(uc.Root))
		}
		for _, mod := range tpl.Modifiers {
			if mod.Condition != nil {
				check(where+" func "+mod.Stat, effect.ItemCondition(*mod.Condition))
			}
			if mod.AttachCondition != nil {
				check(where+" func "+mod.Stat+" attach", effect.ItemCondition(mod.AttachCondition.Root))
			}
		}
	}
	if checked == 0 {
		t.Fatal("no shipped condition checked")
	}

	// The item loader drops a top-level or <for>-leading <cond> that holds no
	// condition, so the walk above cannot see one: read the raw elements and
	// require each of them to build a condition.
	docs, err := loadXMLDocuments[itemFile](datapackPath(t, filepath.Join("data", "xml", "items")), "item template")
	if err != nil {
		t.Fatalf("loadXMLDocuments: %v", err)
	}
	raw := 0
	rawCond := func(where string, id int32, attrs []encxml.Attr, children []condNode) {
		raw++
		if _, null, err := buildUseCondition(id, attrs, children); err != nil {
			t.Errorf("%s: %v", where, err)
		} else if null {
			t.Errorf("%s: <cond> holds no condition and is dropped", where)
		}
	}
	for _, doc := range docs {
		for _, el := range doc.Data.Items {
			id := newAttrValues(foldAttrs(el.Attrs), "item").int32("id")
			where := fmt.Sprintf("%s item %d", filepath.Base(doc.Path), id)
			for _, c := range el.Cond {
				rawCond(where+" cond", id, c.Attrs, c.Children)
			}
			for _, forEl := range el.For {
				for i, op := range forEl.Ops {
					if tag := op.XMLName.Local; strings.EqualFold(tag, "cond") && leadsWithCond(tag, i, forEl.LeadingNode) {
						rawCond(where+" for cond", id, op.Attrs, op.Children)
					}
				}
			}
		}
	}
	if raw == 0 {
		t.Fatal("no shipped item <cond> read")
	}
}

// TestShippedItemCondGuardSeesDroppedCond: the raw <cond> check of
// TestShippedConditionsHoldNoNullNode reports each <cond> the item loader
// drops as null, which a walk over the loaded templates cannot see.
func TestShippedItemCondGuardSeesDroppedCond(t *testing.T) {
	t.Parallel()
	leaf := func(kind string, attrs ...encxml.Attr) condNode {
		return condNode{XMLName: encxml.Name{Local: kind}, Attrs: attrs}
	}
	lvl := encxml.Attr{Name: encxml.Name{Local: "lvl"}, Value: "40"}
	for name, children := range map[string][]condNode{
		"no predicate":      nil,
		"unknown attribute": {leaf("player", lvl)},
		"unknown element":   {leaf("bogus")},
		"empty not":         {leaf("not")},
	} {
		attrs := []encxml.Attr{{Name: encxml.Name{Local: "msgId"}, Value: "113"}}
		if _, null, err := buildUseCondition(1, attrs, children); err != nil || !null {
			t.Errorf("%s: null = %v, err = %v; want a dropped null <cond>", name, null, err)
		}
	}
}

// nullNodes lists the nodes of node the null-condition rules read
// differently from a strict reader.
func nullNodes(node skill.Condition) []string {
	var out []string
	if conditions.IsNull(node) {
		out = append(out, fmt.Sprintf("<%s %v> holds no condition", node.Kind, node.Attrs))
	}
	switch strings.ToLower(node.Kind) {
	case "not":
		if len(node.Children) != 1 {
			out = append(out, fmt.Sprintf("<not> has %d children", len(node.Children)))
		}
	case "player", "target", "using", "game":
		for name, val := range node.Attrs {
			one := skill.Condition{Kind: node.Kind, Attrs: map[string]string{name: val}, Children: node.Children}
			if conditions.IsNull(one) {
				out = append(out, fmt.Sprintf("<%s %s=%q> is not recognized", node.Kind, name, val))
			}
		}
	}
	for _, ch := range node.Children {
		if strings.EqualFold(node.Kind, "player") {
			continue // the <zone> of an insidePoly is no condition
		}
		out = append(out, nullNodes(ch)...)
	}
	return out
}
