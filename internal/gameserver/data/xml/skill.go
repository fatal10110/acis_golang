package xml

import (
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// skillFile is the root <list> element of one skill definition XML file.
type skillFile struct {
	Skills []skillElement `xml:"skill"`
}

// skillElement is one <skill> element: its own id/name/level-count
// attributes, a set of per-level substitution tables, and the <set>,
// <enchant1> and <enchant2> children that carry the actual attribute values
// (a level's value may reference a table by "#name" instead of a literal).
type skillElement struct {
	ID             string `xml:"id,attr"`
	Name           string `xml:"name,attr"`
	Levels         string `xml:"levels,attr"`
	EnchantLevels1 string `xml:"enchantLevels1,attr"`
	EnchantLevels2 string `xml:"enchantLevels2,attr"`

	Tables   []tableElement `xml:"table"`
	Sets     []setElem      `xml:"set"`
	Enchant1 []setElem      `xml:"enchant1"`
	Enchant2 []setElem      `xml:"enchant2"`

	Cond         []condElement `xml:"cond"`
	For          []forElement  `xml:"for"`
	Enchant1Cond []condElement `xml:"enchant1cond"`
	Enchant1For  []forElement  `xml:"enchant1for"`
	Enchant2Cond []condElement `xml:"enchant2cond"`
	Enchant2For  []forElement  `xml:"enchant2for"`
}

// LoadSkillDefinitions parses every ".xml" skill definition file directly
// under dir and returns a lookup table of the resulting definitions, keyed
// by id and level. A directory that can't be listed, a file whose XML is
// not well-formed, or a <skill> element with a missing, mangled, or
// out-of-range attribute fails the whole load: the caller gets an error
// rather than a partially populated table.
func LoadSkillDefinitions(dir string) (*skill.Table, error) {
	docs, err := loadXMLDocuments[skillFile](dir, "skill definition")
	if err != nil {
		return nil, err
	}

	var defs []skill.Definition
	for _, doc := range docs {
		for _, el := range doc.Data.Skills {
			parsed, err := buildSkillDefinitions(el)
			if err != nil {
				return nil, fmt.Errorf("xml: %s: %w", doc.Path, err)
			}
			defs = append(defs, parsed...)
		}
	}

	return skill.NewTable(defs), nil
}

// buildSkillDefinitions expands one <skill> element into one Definition per
// regular level (1..levels) and per enchant level (101.. and 141.. when the
// element declares enchantLevels1/2).
func buildSkillDefinitions(el skillElement) ([]skill.Definition, error) {
	rawID, err := strconv.ParseInt(el.ID, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("skill id %q: %w", el.ID, err)
	}
	id := skill.ID(rawID)

	levels, err := strconv.Atoi(el.Levels)
	if err != nil {
		return nil, fmt.Errorf("skill %d: levels %q: %w", id, el.Levels, err)
	}
	enchant1, err := parseCountAttr(el.EnchantLevels1)
	if err != nil {
		return nil, fmt.Errorf("skill %d: enchantLevels1: %w", id, err)
	}
	enchant2, err := parseCountAttr(el.EnchantLevels2)
	if err != nil {
		return nil, fmt.Errorf("skill %d: enchantLevels2: %w", id, err)
	}

	tables, err := buildValueTables(el.Tables)
	if err != nil {
		return nil, fmt.Errorf("skill %d: %w", id, err)
	}

	defs := make([]skill.Definition, 0, levels+enchant1+enchant2)

	for i := 1; i <= levels; i++ {
		set, err := resolveSkillLevel(tables, el.Sets, i)
		if err != nil {
			return nil, fmt.Errorf("skill %d level %d: %w", id, i, err)
		}
		def, err := skill.NewDefinition(id, i, el.Name, set)
		if err != nil {
			return nil, fmt.Errorf("skill %d level %d: %w", id, i, err)
		}
		if err := applySkillTemplates(&def, tables, el.Cond, el.For, i, i); err != nil {
			return nil, fmt.Errorf("skill %d level %d: %w", id, i, err)
		}
		defs = append(defs, def)
	}

	// An enchant level's <set>-sourced values reuse the last regular
	// level's table row; only its <enchantN> values vary per enchant level.
	for i := 0; i < enchant1; i++ {
		level := i + 101
		set, err := resolveSkillLevel(tables, el.Sets, levels)
		if err != nil {
			return nil, fmt.Errorf("skill %d level %d: %w", id, level, err)
		}
		if err := applySkillAttrs(set, tables, el.Enchant1, i+1); err != nil {
			return nil, fmt.Errorf("skill %d level %d: %w", id, level, err)
		}
		def, err := skill.NewDefinition(id, level, el.Name, set)
		if err != nil {
			return nil, fmt.Errorf("skill %d level %d: %w", id, level, err)
		}
		condIndex := i + 1
		conds := el.Enchant1Cond
		if len(conds) == 0 {
			condIndex = levels
			conds = el.Cond
		}
		forIndex := i + 1
		fors := el.Enchant1For
		if len(fors) == 0 {
			forIndex = levels
			fors = el.For
		}
		if err := applySkillTemplates(&def, tables, conds, fors, condIndex, forIndex); err != nil {
			return nil, fmt.Errorf("skill %d level %d: %w", id, level, err)
		}
		defs = append(defs, def)
	}

	for i := 0; i < enchant2; i++ {
		level := i + 141
		set, err := resolveSkillLevel(tables, el.Sets, levels)
		if err != nil {
			return nil, fmt.Errorf("skill %d level %d: %w", id, level, err)
		}
		if err := applySkillAttrs(set, tables, el.Enchant2, i+1); err != nil {
			return nil, fmt.Errorf("skill %d level %d: %w", id, level, err)
		}
		def, err := skill.NewDefinition(id, level, el.Name, set)
		if err != nil {
			return nil, fmt.Errorf("skill %d level %d: %w", id, level, err)
		}
		condIndex := i + 1
		conds := el.Enchant2Cond
		if len(conds) == 0 {
			condIndex = levels
			conds = el.Cond
		}
		forIndex := i + 1
		fors := el.Enchant2For
		if len(fors) == 0 {
			forIndex = levels
			fors = el.For
		}
		if err := applySkillTemplates(&def, tables, conds, fors, condIndex, forIndex); err != nil {
			return nil, fmt.Errorf("skill %d level %d: %w", id, level, err)
		}
		defs = append(defs, def)
	}

	return defs, nil
}

// resolveSkillLevel builds the StatSet for one level by applying attrs in
// order, resolving any table-referencing value against row tableIndex (the
// level within the referenced table, 1-based).
func resolveSkillLevel(tables map[string][]string, attrs []setElem, tableIndex int) (*commons.StatSet, error) {
	set := commons.NewStatSetWithCapacity(len(attrs))
	if err := applySkillAttrs(set, tables, attrs, tableIndex); err != nil {
		return nil, err
	}
	return set, nil
}

// applySkillAttrs applies attrs to set in order, resolving any
// table-referencing value ("#name") against row tableIndex and overwriting
// whatever the same attribute name already held.
func applySkillAttrs(set *commons.StatSet, tables map[string][]string, attrs []setElem, tableIndex int) error {
	for _, a := range attrs {
		v, err := resolveTableValue(tables, a.Name, a.Val, tableIndex)
		if err != nil {
			return err
		}
		set.Set(a.Name, v)
	}
	return nil
}

// parseCountAttr parses an optional level-count attribute ("enchantLevels1",
// "enchantLevels2"), defaulting to 0 when the element omits it.
func parseCountAttr(s string) (int, error) {
	if s == "" {
		return 0, nil
	}
	return strconv.Atoi(s)
}

func applySkillTemplates(def *skill.Definition, tables map[string][]string, conds []condElement, fors []forElement, condIndex, forIndex int) error {
	for _, c := range conds {
		clause, err := buildSkillConditionClause(tables, c.Attrs, c.Children, condIndex)
		if err != nil {
			return err
		}
		def.Conditions = append(def.Conditions, clause)
	}
	for _, f := range fors {
		if err := applyTemplateNodes(def, tables, f.Ops, forIndex); err != nil {
			return err
		}
	}
	return nil
}

func applyTemplateNodes(def *skill.Definition, tables map[string][]string, ops []funcElement, tableIndex int) error {
	var attachCond *skill.ConditionClause
	for _, op := range ops {
		if strings.EqualFold(op.XMLName.Local, "cond") {
			clause, err := buildSkillConditionClause(tables, op.Attrs, op.Children, tableIndex)
			if err != nil {
				return err
			}
			attachCond = &clause
			continue
		}
		if strings.EqualFold(op.XMLName.Local, "effect") {
			eff, err := buildSkillEffect(tables, op, attachCond, tableIndex)
			if err != nil {
				return err
			}
			if eff.Self {
				def.SelfEffects = append(def.SelfEffects, eff)
			} else {
				def.Effects = append(def.Effects, eff)
			}
			continue
		}

		fn, err := buildSkillFunc(tables, op.XMLName.Local, op.Attrs, op.Children, attachCond, tableIndex)
		if err != nil {
			return err
		}
		def.Funcs = append(def.Funcs, fn)
	}
	return nil
}

func buildSkillEffect(tables map[string][]string, op funcElement, attachCond *skill.ConditionClause, tableIndex int) (skill.EffectTemplate, error) {
	attrs, err := resolvedAttrs(tables, op.Attrs, tableIndex)
	if err != nil {
		return skill.EffectTemplate{}, err
	}
	set := commons.StatSetFromXMLAttrs(attrs)
	base := commons.NewFields(set, "effect")
	name := base.String("name")
	if err := base.Err(); err != nil {
		return skill.EffectTemplate{}, err
	}
	f := commons.NewFields(set, "effect "+name)
	value := f.Float64("val")
	count := f.Int32LiteralDefault("count", 1)
	time := f.Int32LiteralDefault("time", 1)
	self := f.Int32LiteralDefault("self", 0)
	noIcon := f.Int32LiteralDefault("noicon", 0)
	stackOrder := f.Float64Default("stackOrder", 0)
	effectPower := f.Float64Default("effectPower", -1)
	triggeredID := f.Int32LiteralDefault("triggeredId", 0)
	triggeredLevel := f.Int32LiteralDefault("triggeredLevel", 1)
	activationChance := f.Int32LiteralDefault("activationChance", -1)
	if err := f.Err(); err != nil {
		return skill.EffectTemplate{}, err
	}
	eff := skill.EffectTemplate{
		Name:             name,
		Value:            value,
		Count:            int(count),
		Time:             int(time),
		Self:             self == 1,
		Icon:             noIcon != 1,
		Abnormal:         f.StringDefault("abnormal", "NULL"),
		StackType:        f.StringDefault("stackType", "none"),
		StackOrder:       stackOrder,
		EffectPower:      effectPower,
		EffectType:       f.StringDefault("effectType", ""),
		TriggeredID:      int(triggeredID),
		TriggeredLevel:   int(triggeredLevel),
		ChanceType:       f.StringDefault("chanceType", ""),
		ActivationChance: int(activationChance),
		AttachCondition:  attachCond,
	}
	if err := buildNestedEffectTemplates(&eff, tables, op.Children, tableIndex); err != nil {
		return skill.EffectTemplate{}, fmt.Errorf("effect %s: %w", name, err)
	}
	return eff, nil
}

func buildNestedEffectTemplates(eff *skill.EffectTemplate, tables map[string][]string, nodes []condNode, tableIndex int) error {
	var attachCond *skill.ConditionClause
	for _, n := range nodes {
		if strings.EqualFold(n.XMLName.Local, "cond") {
			clause, err := buildSkillConditionClause(tables, n.Attrs, n.Children, tableIndex)
			if err != nil {
				return err
			}
			attachCond = &clause
			continue
		}
		fnEl := funcElement{XMLName: n.XMLName, Attrs: n.Attrs, Children: n.Children}
		fn, err := buildSkillFunc(tables, n.XMLName.Local, fnEl.Attrs, fnEl.Children, attachCond, tableIndex)
		if err != nil {
			return err
		}
		eff.Funcs = append(eff.Funcs, fn)
	}
	return nil
}

func buildSkillFunc(tables map[string][]string, tag string, attrs []xml.Attr, children []condNode, attachCond *skill.ConditionClause, tableIndex int) (skill.FuncTemplate, error) {
	op, err := skill.ParseFuncOp(tag)
	if err != nil {
		return skill.FuncTemplate{}, err
	}
	resolved, err := resolvedAttrs(tables, attrs, tableIndex)
	if err != nil {
		return skill.FuncTemplate{}, err
	}
	set := commons.StatSetFromXMLAttrs(resolved)
	stat, err := set.GetString("stat")
	if err != nil {
		return skill.FuncTemplate{}, fmt.Errorf("%s: %w", tag, err)
	}
	value, err := set.GetFloat64("val")
	if err != nil {
		return skill.FuncTemplate{}, fmt.Errorf("%s %s: %w", tag, stat, err)
	}
	fn := skill.FuncTemplate{Op: op, Stat: stat, Value: value, AttachCondition: attachCond}
	if len(children) > 0 {
		cond, err := buildSkillCondition(tables, children[0], tableIndex)
		if err != nil {
			return skill.FuncTemplate{}, err
		}
		fn.Condition = &cond
	}
	return fn, nil
}

func buildSkillConditionClause(tables map[string][]string, attrs []xml.Attr, children []condNode, tableIndex int) (skill.ConditionClause, error) {
	if len(children) == 0 {
		return skill.ConditionClause{}, fmt.Errorf("cond: no predicate defined")
	}
	resolved, err := resolvedAttrs(tables, attrs, tableIndex)
	if err != nil {
		return skill.ConditionClause{}, err
	}
	set := commons.StatSetFromXMLAttrs(resolved)
	root, err := buildSkillCondition(tables, children[0], tableIndex)
	if err != nil {
		return skill.ConditionClause{}, err
	}
	clause := skill.ConditionClause{Root: root}
	f := commons.NewFields(set, "cond")
	clause.Message = f.StringDefault("msg", "")
	clause.MessageID = f.Int32LiteralDefault("msgId", 0)
	clause.AddName = f.Has("addName") && clause.MessageID > 0
	if err := f.Err(); err != nil {
		return skill.ConditionClause{}, err
	}
	return clause, nil
}

func buildSkillCondition(tables map[string][]string, n condNode, tableIndex int) (skill.Condition, error) {
	attrs := make(map[string]string, len(n.Attrs))
	for _, a := range n.Attrs {
		v, err := resolveTableValue(tables, a.Name.Local, a.Value, tableIndex)
		if err != nil {
			return skill.Condition{}, err
		}
		attrs[a.Name.Local] = v
	}
	var children []skill.Condition
	for _, c := range n.Children {
		child, err := buildSkillCondition(tables, c, tableIndex)
		if err != nil {
			return skill.Condition{}, err
		}
		children = append(children, child)
	}
	return skill.Condition{
		Kind:     strings.ToLower(n.XMLName.Local),
		Attrs:    attrs,
		Children: children,
	}, nil
}

func resolvedAttrs(tables map[string][]string, attrs []xml.Attr, tableIndex int) ([]xml.Attr, error) {
	resolved := attrs
	copied := false
	for i, a := range attrs {
		v, err := resolveTableValue(tables, a.Name.Local, a.Value, tableIndex)
		if err != nil {
			return nil, err
		}
		if v != a.Value {
			if !copied {
				resolved = append([]xml.Attr(nil), attrs...)
				copied = true
			}
			resolved[i].Value = v
		}
	}
	return resolved, nil
}
