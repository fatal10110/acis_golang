package xml

import (
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/rs/zerolog"
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
// by id and level. A directory that can't be listed or a file whose XML is
// not well-formed fails the whole load: the caller gets an error rather
// than a partially populated table. A <skill> element with a missing,
// mangled, or out-of-range attribute is logged and skipped, matching
// DocumentSkill.java's per-level try/catch ("Failed parsing skill."); other
// skills and files continue loading.
//
// log receives skipped-skill diagnostics; the zero logger discards them.
func LoadSkillDefinitions(dir string, log zerolog.Logger) (*skill.Table, error) {
	docs, err := loadXMLDocuments[skillFile](dir, "skill definition")
	if err != nil {
		return nil, err
	}

	var defs []skill.Definition
	for _, doc := range docs {
		for _, el := range doc.Data.Skills {
			parsed, err := buildSkillDefinitions(el)
			if err != nil {
				log.Error().Err(err).Str("file", doc.Path).Msg("data/xml: skipping malformed skill definition")
				continue
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
		vals, err := resolveSkillLevel(tables, el.Sets, i)
		if err != nil {
			return nil, fmt.Errorf("skill %d level %d: %w", id, i, err)
		}
		attrs, err := buildSkillDefinitionAttrs(id, i, vals)
		if err != nil {
			return nil, err
		}
		def := skill.NewDefinition(id, i, el.Name, attrs)
		if err := applySkillTemplates(&def, tables, el.Cond, el.For, i, i, condMsgModeRegular); err != nil {
			return nil, fmt.Errorf("skill %d level %d: %w", id, i, err)
		}
		defs = append(defs, def)
	}

	// An enchant level's <set>-sourced values reuse the last regular
	// level's table row; only its <enchantN> values vary per enchant level.
	for i := 0; i < enchant1; i++ {
		level := i + 101
		vals, err := resolveSkillLevel(tables, el.Sets, levels)
		if err != nil {
			return nil, fmt.Errorf("skill %d level %d: %w", id, level, err)
		}
		if err := applySkillAttrs(vals, tables, el.Enchant1, i+1); err != nil {
			return nil, fmt.Errorf("skill %d level %d: %w", id, level, err)
		}
		attrs, err := buildSkillDefinitionAttrs(id, level, vals)
		if err != nil {
			return nil, err
		}
		def := skill.NewDefinition(id, level, el.Name, attrs)
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
		if err := applySkillTemplates(&def, tables, conds, fors, condIndex, forIndex, condMsgModeEnchant); err != nil {
			return nil, fmt.Errorf("skill %d level %d: %w", id, level, err)
		}
		defs = append(defs, def)
	}

	for i := 0; i < enchant2; i++ {
		level := i + 141
		vals, err := resolveSkillLevel(tables, el.Sets, levels)
		if err != nil {
			return nil, fmt.Errorf("skill %d level %d: %w", id, level, err)
		}
		if err := applySkillAttrs(vals, tables, el.Enchant2, i+1); err != nil {
			return nil, fmt.Errorf("skill %d level %d: %w", id, level, err)
		}
		attrs, err := buildSkillDefinitionAttrs(id, level, vals)
		if err != nil {
			return nil, err
		}
		def := skill.NewDefinition(id, level, el.Name, attrs)
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
		if err := applySkillTemplates(&def, tables, conds, fors, condIndex, forIndex, condMsgModeEnchant); err != nil {
			return nil, fmt.Errorf("skill %d level %d: %w", id, level, err)
		}
		defs = append(defs, def)
	}

	return defs, nil
}

// resolveSkillLevel builds the raw attribute values for one level by
// applying attrs in order, resolving any table-referencing value against row
// tableIndex (the level within the referenced table, 1-based).
func resolveSkillLevel(tables map[string][]string, attrs []setElem, tableIndex int) (map[string]string, error) {
	vals := make(map[string]string, len(attrs))
	if err := applySkillAttrs(vals, tables, attrs, tableIndex); err != nil {
		return nil, err
	}
	return vals, nil
}

// applySkillAttrs applies attrs to vals in order, resolving any
// table-referencing value ("#name") against row tableIndex and overwriting
// whatever the same attribute name already held.
func applySkillAttrs(vals map[string]string, tables map[string][]string, attrs []setElem, tableIndex int) error {
	for _, a := range attrs {
		v, err := resolveTableValue(tables, a.Name, a.Val, tableIndex)
		if err != nil {
			return err
		}
		vals[a.Name] = v
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

func applySkillTemplates(def *skill.Definition, tables map[string][]string, conds []condElement, fors []forElement, condIndex, forIndex int, msgMode condMsgMode) error {
	for _, c := range conds {
		clause, err := buildSkillConditionClause(tables, c.Attrs, c.Children, condIndex, msgMode)
		if err != nil {
			return err
		}
		if clause == nil {
			continue
		}
		def.Conditions = append(def.Conditions, *clause)
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
			clause, err := buildSkillConditionClause(tables, op.Attrs, op.Children, tableIndex, condMsgModeBoth)
			if err != nil {
				return err
			}
			attachCond = clause
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

		// DocumentBase.java's parseTemplate has no fall-through branch for an
		// unrecognized tag inside a <for> block: it silently skips it rather
		// than failing the file (DocumentBase.java:145-174).
		if _, err := skill.ParseFuncOp(op.XMLName.Local); err != nil {
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
	vals, err := resolveAttrMap(tables, op.Attrs, tableIndex)
	if err != nil {
		return skill.EffectTemplate{}, err
	}
	a := newAttrValues(vals, "effect")
	name := a.str("name")
	if err := a.Err(); err != nil {
		return skill.EffectTemplate{}, err
	}
	a.prefix = "effect " + name

	eff := skill.EffectTemplate{
		Name:             name,
		Value:            a.float64("val"),
		Count:            int(a.int32LiteralDefault("count", 1)),
		Time:             int(a.int32LiteralDefault("time", 1)),
		Self:             a.int32LiteralDefault("self", 0) == 1,
		Icon:             a.int32LiteralDefault("noicon", 0) != 1,
		Abnormal:         a.strDefault("abnormal", "NULL"),
		StackType:        a.strDefault("stackType", "none"),
		StackOrder:       a.float64Default("stackOrder", 0),
		EffectPower:      a.float64Default("effectPower", -1),
		EffectPowerSet:   a.has("effectPower"),
		EffectType:       a.strDefault("effectType", ""),
		TriggeredID:      int(a.int32LiteralDefault("triggeredId", 0)),
		TriggeredLevel:   int(a.int32LiteralDefault("triggeredLevel", 1)),
		ChanceType:       a.strDefault("chanceType", ""),
		ActivationChance: int(a.int32LiteralDefault("activationChance", -1)),
		AttachCondition:  attachCond,
	}
	if err := a.Err(); err != nil {
		return skill.EffectTemplate{}, err
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
			clause, err := buildSkillConditionClause(tables, n.Attrs, n.Children, tableIndex, condMsgModeBoth)
			if err != nil {
				return err
			}
			attachCond = clause
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
	vals, err := resolveAttrMap(tables, attrs, tableIndex)
	if err != nil {
		return skill.FuncTemplate{}, err
	}
	a := newAttrValues(vals, tag)
	stat := a.str("stat")
	if err := a.Err(); err != nil {
		return skill.FuncTemplate{}, err
	}
	a.prefix = tag + " " + stat
	value := a.float64("val")
	if err := a.Err(); err != nil {
		return skill.FuncTemplate{}, err
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

// condMsgMode selects how buildSkillConditionClause resolves a cond's
// msg/msgId/addName attributes, mirroring the differences between the three
// places DocumentSkill.java attaches a message to a condition:
//   - condMsgModeRegular: a regular-level <cond> (DocumentSkill.java:216-224)
//     uses msg when present, else msgId (with addName only when msgId > 0);
//     msgId and addName are never read alongside msg.
//   - condMsgModeEnchant: an enchant1cond/enchant2cond block
//     (DocumentSkill.java:245-247, 289-291) reads only msg — msgId and
//     addName are never consulted, even when present in the XML.
//   - condMsgModeBoth: op-level <cond> attachment (DocumentBase.java's
//     generic parseTemplate, not DocumentSkill's per-level loop) has no
//     message semantics in the reference at all; this preserves the prior
//     Go behavior of resolving msg and msgId independently rather than
//     changing untested behavior outside this finding's scope.
type condMsgMode int

const (
	condMsgModeRegular condMsgMode = iota
	condMsgModeEnchant
	condMsgModeBoth
)

// buildSkillConditionClause resolves a <cond> element into a clause. A cond
// with no predicate child returns (nil, nil): DocumentBase.java's
// parseCondition returns null for a missing element node, and attach(null)
// is a tolerated no-op (DocumentBase.java:309-315,341) rather than a load
// failure.
func buildSkillConditionClause(tables map[string][]string, attrs []xml.Attr, children []condNode, tableIndex int, msgMode condMsgMode) (*skill.ConditionClause, error) {
	if len(children) == 0 {
		return nil, nil
	}
	vals, err := resolveAttrMap(tables, attrs, tableIndex)
	if err != nil {
		return nil, err
	}
	root, err := buildSkillCondition(tables, children[0], tableIndex)
	if err != nil {
		return nil, err
	}
	a := newAttrValues(vals, "cond")
	clause := skill.ConditionClause{Root: root}
	switch msgMode {
	case condMsgModeRegular:
		if a.has("msg") {
			clause.Message = a.strDefault("msg", "")
		} else if a.has("msgId") {
			clause.MessageID = a.int32LiteralDefault("msgId", 0)
			clause.AddName = a.has("addName") && clause.MessageID > 0
		}
	case condMsgModeEnchant:
		clause.Message = a.strDefault("msg", "")
	case condMsgModeBoth:
		clause.Message = a.strDefault("msg", "")
		clause.MessageID = a.int32LiteralDefault("msgId", 0)
		clause.AddName = a.has("addName") && clause.MessageID > 0
	}
	if err := a.Err(); err != nil {
		return nil, err
	}
	return &clause, nil
}

func buildSkillCondition(tables map[string][]string, n condNode, tableIndex int) (skill.Condition, error) {
	attrs, err := resolveAttrMap(tables, n.Attrs, tableIndex)
	if err != nil {
		return skill.Condition{}, err
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

// buildSkillDefinitionAttrs resolves one level's raw <set> values into the
// typed attributes skill.NewDefinition takes. Every attribute is parsed and
// defaulted here: a level's values come from a per-level substitution of one
// shared attribute list, so there is no fixed element per attribute for the
// model package to decode itself.
func buildSkillDefinitionAttrs(id skill.ID, level int, vals map[string]string) (skill.DefinitionAttrs, error) {
	a := newAttrValues(vals, fmt.Sprintf("skill %d level %d", id, level))

	attrs := skill.DefinitionAttrs{
		Activation: attrEnum(a, "operateType", skill.ParseActivation),
		Magic:      a.boolDefault("isMagic", false),
		Potion:     a.boolDefault("isPotion", false),

		MPConsume:        a.intDefault("mpConsume", 0),
		MPInitialConsume: a.intDefault("mpInitialConsume", 0),
		HPConsume:        a.intDefault("hpConsume", 0),

		TargetConsumeCount: a.intDefault("targetConsumeCount", 0),
		TargetConsumeID:    a.intDefault("targetConsumeId", 0),
		ItemConsumeCount:   a.intDefault("itemConsumeCount", 0),
		ItemConsumeID:      a.intDefault("itemConsumeId", 0),

		CastRange:           a.intDefault("castRange", 0),
		EffectRange:         a.intDefault("effectRange", -1),
		AbnormalLevel:       a.intDefault("abnormalLvl", -1),
		EffectAbnormalLevel: a.intDefault("effectAbnormalLvl", -1),
		NegateLevel:         a.intDefault("negateLvl", -1),

		HitTime:    a.intDefault("hitTime", 0),
		CoolTime:   a.intDefault("coolTime", 0),
		ReuseDelay: a.intDefault("reuseDelay", 0),
		EquipDelay: a.intDefault("equipDelay", 0),

		Radius: a.intDefault("skillRadius", 80),

		Target: attrEnum(a, "target", skill.ParseTarget),
		Power:  a.float32Default("power", 0),

		Attribute: a.strDefault("attribute", ""),

		MaxNegatedEffects: a.intDefault("maxNegated", 0),
		MagicLevel:        a.intDefault("magicLvl", 0),
		LevelDepend:       a.intDefault("lvlDepend", 0),
		IgnoreResists:     a.boolDefault("ignoreResists", false),
		StaticReuse:       a.boolDefault("staticReuse", false),
		StaticHitTime:     a.boolDefault("staticHitTime", false),

		Stat:         a.strDefault("stat", ""),
		IgnoreShield: a.boolDefault("ignoreShld", false),

		SkillType:  a.str("skillType"),
		EffectType: a.strDefault("effectType", ""),

		EffectID:    a.intDefault("effectId", 0),
		EffectPower: a.intDefault("effectPower", 0),
		EffectLevel: a.intDefault("effectLevel", 0),
		EffectNpcID: a.intDefault("effectNpcId", -1),

		Element:      attrEnumDefault(a, "element", skill.ParseElement, skill.ElementNone),
		BaseLandRate: a.intDefault("baseLandRate", 0),

		Overhit:          a.boolDefault("overHit", false),
		KillByDOT:        a.boolDefault("killByDOT", false),
		SuicideAttack:    a.boolDefault("isSuicideAttack", false),
		SiegeSummonSkill: a.boolDefault("isSiegeSummonSkill", false),

		IsCubic: a.boolDefault("isCubic", false),
		NpcID:   a.intDefault("npcId", 0),

		CubicActivationTime:   a.intDefault("activationtime", 8),
		CubicActivationChance: a.intDefault("activationchance", 30),
		SummonTotalLifeTime:   a.intDefault("summonTotalLifeTime", 1200000),

		WeaponsAllowed: a.strDefault("weaponsAllowed", ""),

		NextActionIsAttack: a.boolDefault("nextActionAttack", false),
		MinPledgeClass:     a.intDefault("minPledgeClass", 0),

		TriggeredID:      a.intDefault("triggeredId", 0),
		TriggeredLevel:   a.intDefault("triggeredLevel", 0),
		ChanceType:       a.strDefault("chanceType", ""),
		ActivationChance: a.intDefault("activationChance", -1),

		Debuff:     a.boolDefault("isDebuff", false),
		MaxCharges: a.intDefault("maxCharges", 0),
		NumCharges: a.intDefault("numCharges", 0),

		LethalChance1: a.intDefault("lethal1", 0),
		LethalChance2: a.intDefault("lethal2", 0),

		DirectHPDamage: a.boolDefault("dmgDirectlyToHp", false),
		Dance:          a.boolDefault("isDance", false),
		NextDanceCost:  a.intDefault("nextDanceCost", 0),
		SoulShotBoost:  a.float32Default("SSBoost", 0),
		AggroPoints:    a.intDefault("aggroPoints", 0),

		StayAfterDeath: a.boolDefault("stayAfterDeath", false),

		FlyRadius: a.intDefault("flyRadius", 0),
		FlyCourse: a.float32Default("flyCourse", 0),

		Feed: a.intDefault("feed", 0),

		CanBeReflected:   a.boolDefault("canBeReflected", true),
		CanBeDispelled:   a.boolDefault("canBeDispeled", true),
		ClanSkill:        a.boolDefault("isClanSkill", false),
		SimultaneousCast: a.boolDefault("simultaneousCast", false),

		ExtractableItems: a.strDefault("capsuled_items_skill", ""),
	}

	if negate := a.strDefault("negateStats", ""); negate != "" {
		attrs.NegateTypes = strings.Fields(negate)
	}

	if a.has("sharedReuse") {
		raw := a.str("sharedReuse")
		ref, err := skill.ParseRef(raw)
		if err != nil {
			a.fail(fmt.Errorf("sharedReuse %q: %w", raw, err))
		} else {
			attrs.SharedReuse = &ref
		}
	}

	if a.has("negateId") {
		raw := a.str("negateId")
		ids, err := parseCommaInts(raw)
		if err != nil {
			a.fail(fmt.Errorf("negateId %q: %w", raw, err))
		} else {
			attrs.NegateIDs = ids
		}
	}

	// offensive and baseCritRate stay unset when the level's data omits
	// them, so NewDefinition can derive each from the level's skill type.
	if a.has("offensive") {
		offensive := a.boolDefault("offensive", false)
		attrs.Offensive = &offensive
	}
	if a.has("baseCritRate") {
		rate := a.intDefault("baseCritRate", 0)
		attrs.BaseCritRate = &rate
	}

	if a.has("flyType") {
		flight := attrEnum(a, "flyType", skill.ParseFlight)
		attrs.Flight = &flight
	}

	if err := a.Err(); err != nil {
		return skill.DefinitionAttrs{}, err
	}
	return attrs, nil
}

// parseCommaInts parses a comma-separated list of integers.
func parseCommaInts(raw string) ([]int, error) {
	parts := strings.Split(raw, ",")
	out := make([]int, len(parts))
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil, err
		}
		out[i] = n
	}
	return out, nil
}
