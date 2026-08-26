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
// than a partially populated table. A <skill> element whose id, levels,
// enchant-level count, or <table> block can't even be parsed is logged and
// skipped as a whole element. Within an otherwise-valid element, a single
// level (regular or enchant) that fails to build is logged and skipped on
// its own, matching DocumentSkill.java's per-level try/catch ("Failed
// parsing skill.", makeSkills, DocumentSkill.java:310-370): other levels of
// the same skill, other skills, and other files continue loading.
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
			parsed, err := buildSkillDefinitions(el, doc.Path, log)
			if err != nil {
				log.Error().Err(err).Str("file", doc.Path).Msg("data/xml: skipping malformed skill definition")
				continue
			}
			defs = append(defs, parsed...)
		}
	}

	return skill.NewTable(defs), nil
}

// skillLoader carries the per-<skill>-element context (its id, its
// resolved <table> substitution rows, and the diagnostics log) needed
// throughout one skill element's construction, so a level or a table-value
// lookup deep in the call tree can log and tolerate its own failure without
// every builder function threading those same three values individually.
type skillLoader struct {
	log    zerolog.Logger
	path   string
	id     skill.ID
	tables map[string][]string
}

// resolveTableValue resolves one attribute value against sl.tables,
// matching DocumentSkill.java's getTableValue/getTableValue(name,int)
// (DocumentSkill.java:55-81): an undefined table name or an out-of-range
// row index is logged and read as "" rather than aborting the skill.
func (sl *skillLoader) resolveTableValue(name, val string, tableIndex int) string {
	resolved, err := resolveTableValue(sl.tables, name, val, tableIndex)
	if err != nil {
		sl.log.Error().Err(err).Str("file", sl.path).Int("skill", int(sl.id)).Msg("data/xml: skill table value unresolved, using empty string")
		return ""
	}
	return resolved
}

// resolveAttrMap folds an element's attributes into a name-keyed map,
// resolving any "#name" table reference against row tableIndex first. A
// repeated attribute name keeps the last value.
func (sl *skillLoader) resolveAttrMap(attrs []xml.Attr, tableIndex int) map[string]string {
	vals := make(map[string]string, len(attrs))
	for _, a := range attrs {
		vals[a.Name.Local] = sl.resolveTableValue(a.Name.Local, a.Value, tableIndex)
	}
	return vals
}

// resolveLevel builds the raw attribute values for one level by applying
// attrs in order, resolving any table-referencing value against row
// tableIndex (the level within the referenced table, 1-based).
func (sl *skillLoader) resolveLevel(attrs []setElem, tableIndex int) map[string]string {
	vals := make(map[string]string, len(attrs))
	sl.applyAttrs(vals, attrs, tableIndex)
	return vals
}

// applyAttrs applies attrs to vals in order, resolving any table-referencing
// value ("#name") against row tableIndex and overwriting whatever the same
// attribute name already held.
func (sl *skillLoader) applyAttrs(vals map[string]string, attrs []setElem, tableIndex int) {
	for _, a := range attrs {
		vals[a.Name] = sl.resolveTableValue(a.Name, a.Val, tableIndex)
	}
}

// buildSkillDefinitions expands one <skill> element into one Definition per
// regular level (1..levels) and per enchant level (101.. and 141.. when the
// element declares enchantLevels1/2). The element's own id, levels,
// enchant-level counts and <table> blocks must all parse for any level to
// build at all; a single level's own build failure only drops that level.
func buildSkillDefinitions(el skillElement, path string, log zerolog.Logger) ([]skill.Definition, error) {
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

	sl := &skillLoader{log: log, path: path, id: id, tables: tables}
	defs := make([]skill.Definition, 0, levels+enchant1+enchant2)

	skipLevel := func(level int, err error) {
		log.Error().Err(err).Str("file", path).Int("skill", int(id)).Int("level", level).Msg("data/xml: skipping malformed skill level")
	}

	for i := 1; i <= levels; i++ {
		vals := sl.resolveLevel(el.Sets, i)
		attrs, err := buildSkillDefinitionAttrs(id, i, vals)
		if err != nil {
			skipLevel(i, err)
			continue
		}
		def := skill.NewDefinition(id, i, el.Name, attrs)
		if err := sl.applyTemplates(&def, el.Cond, el.For, i, i, condMsgModeRegular); err != nil {
			skipLevel(i, err)
			continue
		}
		defs = append(defs, def)
	}

	// An enchant level's <set>-sourced values reuse the last regular
	// level's table row; only its <enchantN> values vary per enchant level.
	for i := 0; i < enchant1; i++ {
		level := i + 101
		vals := sl.resolveLevel(el.Sets, levels)
		sl.applyAttrs(vals, el.Enchant1, i+1)
		attrs, err := buildSkillDefinitionAttrs(id, level, vals)
		if err != nil {
			skipLevel(level, err)
			continue
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
		if err := sl.applyTemplates(&def, conds, fors, condIndex, forIndex, condMsgModeEnchant); err != nil {
			skipLevel(level, err)
			continue
		}
		defs = append(defs, def)
	}

	for i := 0; i < enchant2; i++ {
		level := i + 141
		vals := sl.resolveLevel(el.Sets, levels)
		sl.applyAttrs(vals, el.Enchant2, i+1)
		attrs, err := buildSkillDefinitionAttrs(id, level, vals)
		if err != nil {
			skipLevel(level, err)
			continue
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
		if err := sl.applyTemplates(&def, conds, fors, condIndex, forIndex, condMsgModeEnchant); err != nil {
			skipLevel(level, err)
			continue
		}
		defs = append(defs, def)
	}

	return defs, nil
}

// parseCountAttr parses an optional level-count attribute ("enchantLevels1",
// "enchantLevels2"), defaulting to 0 when the element omits it.
func parseCountAttr(s string) (int, error) {
	if s == "" {
		return 0, nil
	}
	return strconv.Atoi(s)
}

func (sl *skillLoader) applyTemplates(def *skill.Definition, conds []condElement, fors []forElement, condIndex, forIndex int, msgMode condMsgMode) error {
	for _, c := range conds {
		clause, err := sl.conditionClause(c.Attrs, c.Children, condIndex, msgMode)
		if err != nil {
			return err
		}
		if clause == nil {
			continue
		}
		def.Conditions = append(def.Conditions, *clause)
	}
	for _, f := range fors {
		if err := sl.applyTemplateNodes(def, f.Ops, forIndex); err != nil {
			return err
		}
	}
	return nil
}

func (sl *skillLoader) applyTemplateNodes(def *skill.Definition, ops []funcElement, tableIndex int) error {
	var attachCond *skill.ConditionClause
	for _, op := range ops {
		if strings.EqualFold(op.XMLName.Local, "cond") {
			clause, err := sl.conditionClause(op.Attrs, op.Children, tableIndex, condMsgModeBoth)
			if err != nil {
				return err
			}
			attachCond = clause
			continue
		}
		if strings.EqualFold(op.XMLName.Local, "effect") {
			eff, err := sl.effect(op, attachCond, tableIndex)
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
		fn, err := sl.funcTemplate(op.XMLName.Local, op.Attrs, op.Children, attachCond, tableIndex)
		if err != nil {
			return err
		}
		def.Funcs = append(def.Funcs, fn)
	}
	return nil
}

func (sl *skillLoader) effect(op funcElement, attachCond *skill.ConditionClause, tableIndex int) (skill.EffectTemplate, error) {
	vals := sl.resolveAttrMap(op.Attrs, tableIndex)
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
	if err := sl.nestedEffectTemplates(&eff, op.Children, tableIndex); err != nil {
		return skill.EffectTemplate{}, fmt.Errorf("effect %s: %w", name, err)
	}
	return eff, nil
}

func (sl *skillLoader) nestedEffectTemplates(eff *skill.EffectTemplate, nodes []condNode, tableIndex int) error {
	var attachCond *skill.ConditionClause
	for _, n := range nodes {
		if strings.EqualFold(n.XMLName.Local, "cond") {
			clause, err := sl.conditionClause(n.Attrs, n.Children, tableIndex, condMsgModeBoth)
			if err != nil {
				return err
			}
			attachCond = clause
			continue
		}
		fnEl := funcElement{XMLName: n.XMLName, Attrs: n.Attrs, Children: n.Children}
		fn, err := sl.funcTemplate(n.XMLName.Local, fnEl.Attrs, fnEl.Children, attachCond, tableIndex)
		if err != nil {
			return err
		}
		eff.Funcs = append(eff.Funcs, fn)
	}
	return nil
}

func (sl *skillLoader) funcTemplate(tag string, attrs []xml.Attr, children []condNode, attachCond *skill.ConditionClause, tableIndex int) (skill.FuncTemplate, error) {
	op, err := skill.ParseFuncOp(tag)
	if err != nil {
		return skill.FuncTemplate{}, err
	}
	vals := sl.resolveAttrMap(attrs, tableIndex)
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
		cond := sl.condition(children[0], tableIndex)
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

// conditionClause resolves a <cond> element into a clause. A cond with no
// predicate child returns (nil, nil): DocumentBase.java's parseCondition
// returns null for a missing element node, and attach(null) is a tolerated
// no-op (DocumentBase.java:309-315,341) rather than a load failure.
func (sl *skillLoader) conditionClause(attrs []xml.Attr, children []condNode, tableIndex int, msgMode condMsgMode) (*skill.ConditionClause, error) {
	if len(children) == 0 {
		return nil, nil
	}
	vals := sl.resolveAttrMap(attrs, tableIndex)
	root := sl.condition(children[0], tableIndex)
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

func (sl *skillLoader) condition(n condNode, tableIndex int) skill.Condition {
	attrs := sl.resolveAttrMap(n.Attrs, tableIndex)
	var children []skill.Condition
	for _, c := range n.Children {
		children = append(children, sl.condition(c, tableIndex))
	}
	return skill.Condition{
		Kind:     strings.ToLower(n.XMLName.Local),
		Attrs:    attrs,
		Children: children,
	}
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
