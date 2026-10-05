package xml

import (
	"encoding/xml"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/conditions"
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
	ID             string  `xml:"id,attr"`
	Name           *string `xml:"name,attr"`
	Levels         string  `xml:"levels,attr"`
	EnchantLevels1 *string `xml:"enchantLevels1,attr"`
	EnchantLevels2 *string `xml:"enchantLevels2,attr"`

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
// than a partially populated table.
//
// Within a file, skills load in order, as the reference loads them. A level
// whose values do not build is logged and dropped with every later level of
// its route (regular, enchant1 or enchant2); see buildLevels. Any other
// failure while a <skill> element is read (its id, level counts or tables, a
// condition, a <for> block, or a template that has no level left to attach
// to) stops the file there: that skill and every later skill in the file
// are logged and not loaded, while the earlier ones are kept.
//
// log receives skipped-skill diagnostics; the zero logger discards them.
func LoadSkillDefinitions(dir string, log zerolog.Logger) (*skill.Table, error) {
	docs, err := loadXMLDocuments[skillFile](dir, "skill definition")
	if err != nil {
		return nil, err
	}

	var defs []skill.Definition
	for _, doc := range docs {
		for i, el := range doc.Data.Skills {
			parsed, err := buildSkillDefinitions(el, doc.Path, log)
			if err != nil {
				log.Error().Err(err).Str("file", doc.Path).Int("skills_not_loaded", len(doc.Data.Skills)-i).
					Msg("data/xml: malformed skill definition stops its file; it and every later skill in the file are not loaded")
				break
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

// resolveTableValue resolves one attribute value against sl.tables: an
// undefined table name or an out-of-range row index is logged and read as
// "" rather than aborting the skill.
func (sl *skillLoader) resolveTableValue(name, val string, tableIndex int) string {
	resolved, err := resolveTableValue(sl.tables, name, val, tableIndex)
	if err != nil {
		sl.log.Error().Err(err).Str("file", sl.path).Int("skill", int(sl.id)).Msg("data/xml: skill table value unresolved, using empty string")
		return ""
	}
	return resolved
}

// resolver returns the table resolver for row tableIndex of the skill's
// tables.
func (sl *skillLoader) resolver(tableIndex int) tableResolver {
	return func(name, val string) string { return sl.resolveTableValue(name, val, tableIndex) }
}

// resolveAttrMap folds an element's attributes into a name-keyed map,
// resolving any "#name" table reference against row tableIndex first,
// except in the attributes named in asWritten, which keep their value. A
// repeated attribute name keeps the last value.
func (sl *skillLoader) resolveAttrMap(attrs []xml.Attr, tableIndex int, asWritten ...string) map[string]string {
	vals := make(map[string]string, len(attrs))
	for _, a := range attrs {
		if slices.Contains(asWritten, a.Name.Local) {
			vals[a.Name.Local] = a.Value
			continue
		}
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
		name, val := strings.TrimSpace(a.Name), strings.TrimSpace(a.Val)
		vals[name] = sl.resolveTableValue(name, val, tableIndex)
	}
}

// buildSkillDefinitions expands one <skill> element into one Definition per
// regular level (1..levels) and per enchant level (101.. and 141.. when the
// element declares enchantLevels1/2). It follows the reference's order: every
// level is built from its values first (buildLevels), then each level's
// conditions and <for> blocks attach to the definition at that level's
// position among the built ones. A dropped level therefore shifts the later
// positions, and a level whose position has no built definition fails the
// element. Any error fails the element, and with it the rest of its file.
func buildSkillDefinitions(el skillElement, path string, log zerolog.Logger) ([]skill.Definition, error) {
	rawID, err := commons.ParseInt(el.ID, 32)
	if err != nil {
		return nil, fmt.Errorf("skill id %q: %w", el.ID, err)
	}
	id := skill.ID(rawID)
	if el.Name == nil {
		return nil, fmt.Errorf("skill %d: attribute \"name\" is missing", id)
	}

	levels, err := parseLevelCount(el.Levels)
	if err != nil {
		return nil, fmt.Errorf("skill %d: levels: %w", id, err)
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
	built := sl.buildLevels(el, levels, enchant1, enchant2)

	for i := 1; i <= levels; i++ {
		if err := sl.applyTemplatesAt(built, i-1, el.Cond, el.For, i, i, condMsgModeRegular); err != nil {
			return nil, fmt.Errorf("skill %d level %d: %w", id, i, err)
		}
	}

	// An enchant route without its own enchantNcond/enchantNfor blocks
	// reads the regular ones against the last regular level's table row.
	for i := 0; i < enchant1; i++ {
		condIndex, conds := i+1, el.Enchant1Cond
		if len(conds) == 0 {
			condIndex, conds = levels, el.Cond
		}
		forIndex, fors := i+1, el.Enchant1For
		if len(fors) == 0 {
			forIndex, fors = levels, el.For
		}
		if err := sl.applyTemplatesAt(built, levels+i, conds, fors, condIndex, forIndex, condMsgModeEnchant); err != nil {
			return nil, fmt.Errorf("skill %d level %d: %w", id, 101+i, err)
		}
	}

	for i := 0; i < enchant2; i++ {
		condIndex, conds := i+1, el.Enchant2Cond
		if len(conds) == 0 {
			condIndex, conds = levels, el.Cond
		}
		forIndex, fors := i+1, el.Enchant2For
		if len(fors) == 0 {
			forIndex, fors = levels, el.For
		}
		if err := sl.applyTemplatesAt(built, levels+enchant1+i, conds, fors, condIndex, forIndex, condMsgModeEnchant); err != nil {
			return nil, fmt.Errorf("skill %d level %d: %w", id, 141+i, err)
		}
	}

	return built, nil
}

// buildLevels builds the definition of every level from its values: the
// regular levels, then the enchant1 levels, then the enchant2 levels. An
// enchant level's <set> values read the last regular level's table row; only
// its <enchantN> values read its own row. A level that does not build is
// logged and dropped with every later level of its route: the reference
// inserts each level at its position within its route, and once one is
// missing, each later insert lands past the end of the list and fails too.
func (sl *skillLoader) buildLevels(el skillElement, levels, enchant1, enchant2 int) []skill.Definition {
	var built []skill.Definition
	route := func(count, first int, vals func(i int) map[string]string) {
		for i := 0; i < count; i++ {
			level := first + i
			attrs, err := buildSkillDefinitionAttrs(sl.id, level, vals(i))
			if err != nil {
				sl.log.Error().Err(err).Str("file", sl.path).Int("skill", int(sl.id)).Int("level", level).Int("levels_not_loaded", count-i).
					Msg("data/xml: malformed skill level; it and every later level of its route are not loaded")
				return
			}
			built = append(built, skill.NewDefinition(sl.id, level, *el.Name, attrs))
		}
	}
	route(levels, 1, func(i int) map[string]string { return sl.resolveLevel(el.Sets, i+1) })
	route(enchant1, 101, func(i int) map[string]string {
		vals := sl.resolveLevel(el.Sets, levels)
		sl.applyAttrs(vals, el.Enchant1, i+1)
		return vals
	})
	route(enchant2, 141, func(i int) map[string]string {
		vals := sl.resolveLevel(el.Sets, levels)
		sl.applyAttrs(vals, el.Enchant2, i+1)
		return vals
	})
	return built
}

// errNoLevelAtPosition marks templates read for a position the built levels
// do not reach.
var errNoLevelAtPosition = errors.New("no built level at this position")

// applyTemplatesAt attaches conds and fors to built[pos]. With neither there
// is nothing to attach and pos is not looked up.
func (sl *skillLoader) applyTemplatesAt(built []skill.Definition, pos int, conds []condElement, fors []forElement, condIndex, forIndex int, msgMode condMsgMode) error {
	if len(conds) == 0 && len(fors) == 0 {
		return nil
	}
	if pos >= len(built) {
		return fmt.Errorf("position %d of %d built levels: %w", pos, len(built), errNoLevelAtPosition)
	}
	return sl.applyTemplates(&built[pos], conds, fors, condIndex, forIndex, msgMode)
}

// parseLevelCount parses a level-count attribute: an int32 that may not be
// negative.
func parseLevelCount(s string) (int, error) {
	n, err := commons.ParseInt(s, 32)
	if err != nil {
		return 0, err
	}
	if n < 0 {
		return 0, fmt.Errorf("%q: negative level count", s)
	}
	return int(n), nil
}

// parseCountAttr parses an optional level-count attribute ("enchantLevels1",
// "enchantLevels2"), defaulting to 0 when the element omits it. A present
// one must parse, even when empty.
func parseCountAttr(s *string) (int, error) {
	if s == nil {
		return 0, nil
	}
	return parseLevelCount(*s)
}

func (sl *skillLoader) applyTemplates(def *skill.Definition, conds []condElement, fors []forElement, condIndex, forIndex int, msgMode condMsgMode) error {
	for _, c := range conds {
		clause, err := conditionClause(c.Attrs, c.Children, sl.resolver(condIndex), msgMode)
		if err != nil {
			return err
		}
		if clause == nil {
			// A skill-level <cond> with no predicate still attaches: its
			// zero Root holds no condition, which refuses every cast.
			clause = &skill.ConditionClause{}
		}
		def.Conditions = append(def.Conditions, *clause)
	}
	for _, f := range fors {
		if err := sl.applyTemplateNodes(def, f, forIndex); err != nil {
			return err
		}
	}
	return nil
}

func (sl *skillLoader) applyTemplateNodes(def *skill.Definition, block forElement, tableIndex int) error {
	var attachCond *skill.ConditionClause
	for i, op := range block.Ops {
		if strings.EqualFold(op.XMLName.Local, "cond") {
			// Only a <cond> that is the block's first node gates it; any
			// other one is never read.
			if !leadsWithCond(op.XMLName.Local, i, block.LeadingNode) {
				continue
			}
			clause, err := conditionClause(op.Attrs, op.Children, sl.resolver(tableIndex), condMsgModeRegular)
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

		// An unrecognized tag inside a <for> block has no fall-through
		// branch: it is silently skipped rather than failing the file.
		if _, err := skill.ParseFuncOp(op.XMLName.Local); err != nil {
			continue
		}
		fn, err := sl.funcTemplate(op.XMLName.Local, op.Attrs, op.Children, attachCond, tableIndex, sl.resolver(tableIndex))
		if err != nil {
			return err
		}
		def.Funcs = append(def.Funcs, fn)
	}
	return nil
}

// effect builds an <effect> of a <for> block. Its values read row
// tableIndex of the skill's tables, except stackType, which is read as
// written.
func (sl *skillLoader) effect(op funcElement, attachCond *skill.ConditionClause, tableIndex int) (skill.EffectTemplate, error) {
	eff, err := readEffectTemplate(sl.resolveAttrMap(op.Attrs, tableIndex, "stackType"))
	if err != nil {
		return skill.EffectTemplate{}, err
	}
	eff.AttachCondition = attachCond
	if err := sl.nestedEffectTemplates(&eff, op, tableIndex); err != nil {
		return skill.EffectTemplate{}, fmt.Errorf("effect %s: %w", eff.Name, err)
	}
	return eff, nil
}

// nestedEffectTemplates reads the children of an <effect>. Their conditions
// belong to the effect, which has no tables, so none of their values may name
// one; the funcs' own values still read the skill's tables. A nested
// <effect> fails; any other unrecognized tag is skipped.
func (sl *skillLoader) nestedEffectTemplates(eff *skill.EffectTemplate, op funcElement, tableIndex int) error {
	var attachCond *skill.ConditionClause
	for i, n := range op.Children {
		tag := n.XMLName.Local
		switch {
		case strings.EqualFold(tag, "cond"):
			// Only a <cond> that is the effect's first node gates it; any
			// other one is never read.
			if !leadsWithCond(tag, i, op.LeadingNode) {
				continue
			}
			clause, err := conditionClause(n.Attrs, n.Children, nil, condMsgModeRegular)
			if err != nil {
				return err
			}
			attachCond = clause
		case strings.EqualFold(tag, "effect"):
			return errNestedEffect
		default:
			if _, err := skill.ParseFuncOp(tag); err != nil {
				continue
			}
			fn, err := sl.funcTemplate(tag, n.Attrs, n.Children, attachCond, tableIndex, nil)
			if err != nil {
				return err
			}
			eff.Funcs = append(eff.Funcs, fn)
		}
	}
	return nil
}

// funcTemplate builds one stat func. Its val reads row tableIndex of the
// skill's tables and its stat is read as written; its condition child
// resolves table references with condResolve.
func (sl *skillLoader) funcTemplate(tag string, attrs []xml.Attr, children []condNode, attachCond *skill.ConditionClause, tableIndex int, condResolve tableResolver) (skill.FuncTemplate, error) {
	op, err := skill.ParseFuncOp(tag)
	if err != nil {
		return skill.FuncTemplate{}, err
	}
	vals := sl.resolveAttrMap(attrs, tableIndex, "stat")
	a := newAttrValues(vals, tag)
	stat := readFuncStat(a)
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
		cond, err := buildSkillCondition(children[0], condRolePredicate, condResolve)
		if err != nil {
			return skill.FuncTemplate{}, fmt.Errorf("%s %s: %w", tag, stat, err)
		}
		fn.Condition = &cond
	}
	return fn, nil
}

// condMsgMode selects how conditionClause reads a cond's msg/msgId/addName
// attributes:
//   - condMsgModeRegular: a regular-level <cond> and a <for>/<effect>
//     block's leading <cond> use msg when present, else msgId (with addName
//     only when msgId > 0); msgId and addName are never read alongside msg.
//   - condMsgModeEnchant: an enchant1cond/enchant2cond block reads only
//     msg — msgId and addName are never consulted, even when present in the
//     XML.
type condMsgMode int

const (
	condMsgModeRegular condMsgMode = iota
	condMsgModeEnchant
)

// conditionClause resolves a <cond> element into a clause, its predicate
// resolving table references with resolve. A cond with no predicate child
// returns (nil, nil). A predicate that holds no condition
// (conditions.IsNull) has no feedback: none of msg, msgId and addName is
// read. Otherwise the cond's own msg is read as written and its msgId may
// not name a table.
func conditionClause(attrs []xml.Attr, children []condNode, resolve tableResolver, msgMode condMsgMode) (*skill.ConditionClause, error) {
	if len(children) == 0 {
		return nil, nil
	}
	root, err := buildSkillCondition(children[0], condRolePredicate, resolve)
	if err != nil {
		return nil, fmt.Errorf("cond: %w", err)
	}
	clause := skill.ConditionClause{Root: root}
	if conditions.IsNull(root) {
		return &clause, nil
	}
	a := newAttrValues(foldAttrs(attrs), "cond")
	if msgMode != condMsgModeEnchant && !a.has("msg") && a.has("msgId") {
		if err := noTableRef("msgId", a.str("msgId")); err != nil {
			return nil, fmt.Errorf("cond: %w", err)
		}
	}
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
	}
	if err := a.Err(); err != nil {
		return nil, err
	}
	return &clause, nil
}

// buildSkillCondition converts one condition element in role into a
// skill.Condition, reading its attributes with conditionAttrs.
func buildSkillCondition(n condNode, role condRole, resolve tableResolver) (skill.Condition, error) {
	attrs, err := conditionAttrs(n, role, resolve)
	if err != nil {
		return skill.Condition{}, fmt.Errorf("<%s>: %w", n.XMLName.Local, err)
	}
	var children []skill.Condition
	for i, c := range n.Children {
		child, err := buildSkillCondition(c, childConditionRole(n, role, i), resolve)
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

		SkillType:  attrSkillType(a),
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
		ExpPenalty:            a.float32Default("expPenalty", 0),

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

		AbsorbPart: a.float32Default("absorbPart", 0),
		AbsorbAbs:  a.intDefault("absorbAbs", 0),

		CanBeReflected:   a.boolDefault("canBeReflected", true),
		CanBeDispelled:   a.boolDefault("canBeDispeled", true),
		ClanSkill:        a.boolDefault("isClanSkill", false),
		SimultaneousCast: a.boolDefault("simultaneousCast", false),

		ExtractableItems: a.strDefault("capsuled_items_skill", ""),

		RecallType: skill.ParseRecallType(a.strDefault("recallType", "")),
	}

	if a.has("teleCoords") {
		if loc, ok := skill.ParseTeleCoords(a.str("teleCoords")); ok {
			attrs.TeleCoords = &loc
		}
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

// attrSkillType reads a level's required skillType, which must name a skill
// type exactly as written.
func attrSkillType(a *attrValues) string {
	name := a.str("skillType")
	if a.err == nil && !skill.KnownSkillType(name) {
		a.fail(fmt.Errorf("attribute %q: unknown skill type %q", "skillType", name))
	}
	return name
}

// parseCommaInts parses a comma-separated list of integers.
func parseCommaInts(raw string) ([]int, error) {
	parts := strings.Split(raw, ",")
	out := make([]int, len(parts))
	for i, p := range parts {
		n, err := commons.Atoi(p)
		if err != nil {
			return nil, err
		}
		out[i] = n
	}
	return out, nil
}
