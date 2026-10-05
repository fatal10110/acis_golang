package xml

import (
	"encoding/xml"
	"fmt"
	"strings"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/conditions"
	"github.com/rs/zerolog"
)

// itemFile is the root element of one item template XML file: a flat list
// of <item> elements.
type itemFile struct {
	Items []itemElement `xml:"item"`
}

// itemElement is one <item> element: its own attributes (id, type, name)
// fold in directly; <set> children flatten alongside them; <table>, <for>
// and <cond> are distinctly shaped child blocks handled by their own types.
type itemElement struct {
	Attrs  []xml.Attr     `xml:",any,attr"`
	Tables []tableElement `xml:"table"`
	Sets   []setElem      `xml:"set"`
	For    []forElement   `xml:"for"`
	Cond   []condElement  `xml:"cond"`
}

// setElem is one <set name="..." val="..."/> attribute-style element.
type setElem struct {
	Name string `xml:"name,attr"`
	Val  string `xml:"val,attr"`
}

// forElement is one <for> block: a flat list of stat-modifier elements
// (<add>, <sub>, <set stat="..." .../>, ...), each captured generically
// since they share one attribute shape and differ only by tag name, plus
// <cond> and <effect>. It decodes itself (see templatenodes.go).
type forElement struct {
	Ops []funcElement
	// LeadingNode reports character data before the first element.
	LeadingNode bool
}

// funcElement is one element inside a <for> block; XMLName carries which
// operation it applies (see item.ParseFuncOp) or names a <cond>/<effect>.
// It decodes itself (see templatenodes.go).
type funcElement struct {
	XMLName  xml.Name
	Attrs    []xml.Attr
	Children []condNode
	// LeadingNode reports character data before the first child element.
	LeadingNode bool
}

// condElement is one <cond> block: its own message attributes plus the
// nested predicate tree that must hold for the item to be usable.
type condElement struct {
	Attrs    []xml.Attr `xml:",any,attr"`
	Children []condNode `xml:",any"`
}

// condNode is one node of a <cond> block's predicate tree (a combinator
// such as <and>, or a leaf predicate such as <player .../>), captured
// generically and recursively since this loader doesn't interpret
// condition semantics — see item.Condition.
type condNode struct {
	XMLName  xml.Name
	Attrs    []xml.Attr `xml:",any,attr"`
	Children []condNode `xml:",any"`
}

// LoadItemTemplates parses every ".xml" item template file directly under
// dir and returns a lookup table of the resulting templates keyed by item
// id. dir is expected to look like a shipped aCis_datapack
// "data/xml/items" directory: one flat list of files, each holding a flat
// list of <item> elements.
//
// A directory that can't be listed or a file whose XML is not well-formed
// fails the whole load: the caller gets an actionable error rather than a
// partially populated table. An individual <item> element that can't be
// turned into a Template is logged and skipped on its own; the rest of the
// file and other files continue loading.
//
// log receives skipped-item diagnostics; the zero logger discards them.
func LoadItemTemplates(dir string, log zerolog.Logger) (*item.Table, error) {
	docs, err := loadXMLDocuments[itemFile](dir, "item template")
	if err != nil {
		return nil, err
	}

	var templates []*item.Template
	for _, doc := range docs {
		for _, el := range doc.Data.Items {
			tpl, err := buildItemTemplate(el)
			if err != nil {
				log.Error().Err(err).Str("file", doc.Path).Msg("data/xml: skipping malformed item template")
				continue
			}
			templates = append(templates, tpl)
		}
	}

	return item.NewTable(templates), nil
}

// buildItemTemplate resolves one parsed <item> element into a Template:
// its own attributes and <set> children fold into one name-keyed value set
// (<set> values resolved against the element's tables), every attribute is
// then read and defaulted there, and the kind-specific detail is built from
// the same resolved values.
func buildItemTemplate(el itemElement) (*item.Template, error) {
	tables, err := buildValueTables(el.Tables)
	if err != nil {
		return nil, err
	}

	vals := foldAttrs(el.Attrs)
	for _, s := range el.Sets {
		val, err := resolveTableValue(tables, s.Name, s.Val, 1)
		if err != nil {
			return nil, err
		}
		vals[s.Name] = val
	}

	a := newAttrValues(vals, "item template")
	id := a.int32("id")
	if err := a.Err(); err != nil {
		return nil, err
	}
	a.prefix = fmt.Sprintf("item template %d", id)

	tpl := &item.Template{
		ID:             id,
		Name:           a.str("name"),
		Weight:         a.int32Default("weight", 0),
		Material:       attrEnumDefault(a, "material", item.ParseMaterialType, item.MaterialSteel),
		Duration:       a.int32Default("duration", -1),
		ReferencePrice: a.int32Default("price", 0),
		Crystal:        attrEnumDefault(a, "crystal_type", item.ParseCrystalType, item.CrystalNone),
		CrystalCount:   a.int32Default("crystal_count", 0),
		Stackable:      a.boolDefault("is_stackable", false),
		Sellable:       a.boolDefault("is_sellable", true),
		Dropable:       a.boolDefault("is_dropable", true),
		Destroyable:    a.boolDefault("is_destroyable", true),
		Tradable:       a.boolDefault("is_tradable", true),
		Depositable:    a.boolDefault("is_depositable", true),
		OlyRestricted:  a.boolDefault("is_oly_restricted", false),
	}
	tpl.Kind = attrEnum(a, "type", item.ParseKind)
	tpl.Slot = attrEnumDefault(a, "bodypart", item.ParseSlot, item.SlotNone)
	tpl.DefaultAction = attrEnumDefault(a, "default_action", item.ParseActionType, item.ActionNone)

	if a.has("item_skill") {
		skills, err := item.ParseSkillRefs(a.strDefault("item_skill", ""))
		if err != nil {
			a.fail(err)
		} else {
			tpl.AttachedSkills = skills
		}
	}

	modifiers, useConditions, err := buildItemClauses(id, el, tables)
	if err != nil {
		return nil, err
	}
	tpl.Modifiers = modifiers
	tpl.UseConditions = useConditions

	switch tpl.Kind {
	case item.KindWeapon:
		tpl.Weapon = buildWeaponDetail(a)
	case item.KindArmor:
		tpl.Armor = item.NewArmorDetail(attrEnumDefault(a, "armor_type", item.ParseArmorType, item.ArmorNone), tpl.Slot)
	case item.KindEtcItem:
		tpl.EtcItem = item.NewEtcItemDetail(attrEnumDefault(a, "etcitem_type", item.ParseEtcItemType, item.EtcItemNone),
			a.strDefault("handler", ""),
			a.int32Default("shared_reuse_group", -1),
			a.int32Default("reuse_delay", 0),
			tpl.DefaultAction)
	}

	if err := a.Err(); err != nil {
		return nil, err
	}
	return tpl, nil
}

// buildWeaponDetail reads a KindWeapon template's weapon-specific attributes
// from a. Every field defaults to its shipped-data default when absent; a
// present-but-malformed value is recorded on a.
func buildWeaponDetail(a *attrValues) *item.WeaponDetail {
	d := &item.WeaponDetail{
		Type:            attrEnumDefault(a, "weapon_type", item.ParseWeaponType, item.WeaponNone),
		SoulshotCount:   a.int32Default("soulshots", 0),
		SpiritshotCount: a.int32Default("spiritshots", 0),
		RandomDamage:    a.int32Default("random_damage", 0),
		MPConsume:       a.int32Default("mp_consume", 0),
	}

	d.MPConsumeReduceRate, d.MPConsumeReduceValue = parseIntPairAttr(a, "mp_consume_reduce")

	d.ReuseDelay = a.int32Default("reuse_delay", 0)
	d.Magical = a.boolDefault("is_magical", false)

	d.ReducedSoulshotChance, d.ReducedSoulshotCount = parseIntPairAttr(a, "reduced_soulshot")

	if a.has("enchant4_skill") {
		ref, err := item.ParseSkillRef(a.strDefault("enchant4_skill", ""))
		if err != nil {
			a.fail(err)
		} else {
			d.Enchant4Skill = &ref
		}
	}

	d.OnCastSkill = parseSkillTriggerAttr(a, "oncast_skill", "oncast_chance")
	d.OnCritSkill = parseSkillTriggerAttr(a, "oncrit_skill", "oncrit_chance")

	return d
}

// parseSkillTriggerAttr reads the optional (skillKey, chanceKey) pair a
// weapon uses to describe an on-cast/on-crit triggered skill: skillKey is an
// "id-level" SkillRef, chanceKey an optional percentage read only when
// skillKey is present (matching the shipped data's own contract: a chance
// value with no skill to gate is never read at all). Returns nil when
// skillKey is absent.
func parseSkillTriggerAttr(a *attrValues, skillKey, chanceKey string) *item.SkillTrigger {
	if a.err != nil || !a.has(skillKey) {
		return nil
	}
	ref, err := item.ParseSkillRef(a.strDefault(skillKey, ""))
	if err != nil {
		a.fail(err)
		return nil
	}

	chance := int32(-1)
	if a.has(chanceKey) {
		chance = a.int32(chanceKey)
	}
	if a.err != nil {
		return nil
	}
	return &item.SkillTrigger{Skill: ref, Chance: chance}
}

// parseIntPairAttr reads key as an "a,b" pair of int32s, returning (0, 0)
// when key is absent. A present value that isn't exactly two comma-separated
// integers is recorded on a.
func parseIntPairAttr(a *attrValues, key string) (int32, int32) {
	if a.err != nil || !a.has(key) {
		return 0, 0
	}
	raw := a.vals[key]
	parts := strings.Split(raw, ",")
	if len(parts) != 2 {
		a.fail(fmt.Errorf("attribute %q: want \"a,b\", got %q", key, raw))
		return 0, 0
	}
	rate, err := commons.ParseInt(strings.TrimSpace(parts[0]), 32)
	if err != nil {
		a.fail(fmt.Errorf("attribute %q: %w", key, err))
		return 0, 0
	}
	value, err := commons.ParseInt(strings.TrimSpace(parts[1]), 32)
	if err != nil {
		a.fail(fmt.Errorf("attribute %q: %w", key, err))
		return 0, 0
	}
	return int32(rate), int32(value)
}

// buildDrop reads one <drop> element's attributes into a Drop. itemid, min,
// max and chance are all required.
func buildDrop(attrs []xml.Attr) (item.Drop, error) {
	a := newAttrValues(foldAttrs(attrs), "drop")
	d := item.Drop{
		ItemID: a.int32Literal("itemid"),
		Min:    a.int32Literal("min"),
		Max:    a.int32Literal("max"),
		Chance: a.float64("chance"),
	}
	if err := a.Err(); err != nil {
		return item.Drop{}, err
	}
	return d, nil
}

// buildDropCategory reads one <category> element's attributes into a
// DropCategory over drops: type is required; chance defaults to 100 when
// absent.
func buildDropCategory(attrs []xml.Attr, drops []item.Drop) (item.DropCategory, error) {
	a := newAttrValues(foldAttrs(attrs), "drop category")
	c := item.DropCategory{
		Kind:   attrEnum(a, "type", item.ParseDropKind),
		Chance: a.float64Default("chance", 100),
		Drops:  drops,
	}
	if err := a.Err(); err != nil {
		return item.DropCategory{}, err
	}
	return c, nil
}

// foldAttrs folds an attribute list into a name-keyed value map, last value
// winning.
func foldAttrs(attrs []xml.Attr) map[string]string {
	vals := make(map[string]string, len(attrs))
	for _, attr := range attrs {
		vals[attr.Name.Local] = attr.Value
	}
	return vals
}

// buildItemClauses builds an <item> element's stat modifiers (from its
// <for> blocks) and use conditions (from its <cond> blocks).
func buildItemClauses(id int32, el itemElement, tables map[string][]string) ([]item.StatModifier, []item.UseCondition, error) {
	var modifiers []item.StatModifier
	for _, forEl := range el.For {
		var attachCond *item.UseCondition
		for i, opEl := range forEl.Ops {
			tag := opEl.XMLName.Local
			if strings.EqualFold(tag, "cond") {
				// Only a <cond> that is the block's first node gates it, and
				// only when its predicate holds a condition; any other one is
				// never read.
				if leadsWithCond(tag, i, forEl.LeadingNode) {
					uc, null, err := buildUseCondition(id, opEl.Attrs, opEl.Children)
					if err != nil {
						return nil, nil, err
					}
					if !null {
						attachCond = &uc
					}
				}
				continue
			}

			if strings.EqualFold(tag, "effect") {
				// An item has no effect templates: the effect is read with
				// the skill grammar, then discarded.
				if err := validateItemEffect(id, opEl, tables); err != nil {
					return nil, nil, err
				}
				continue
			}

			op, err := item.ParseFuncOp(tag)
			if err != nil {
				// Unrecognized <for> children are silently ignored.
				continue
			}
			mod, err := buildItemFunc(id, op, opEl.Attrs, opEl.Children, tables)
			if err != nil {
				return nil, nil, err
			}
			mod.AttachCondition = attachCond
			modifiers = append(modifiers, mod)
		}
	}

	var useConditions []item.UseCondition
	for _, condEl := range el.Cond {
		uc, null, err := buildUseCondition(id, condEl.Attrs, condEl.Children)
		if err != nil {
			return nil, nil, err
		}
		// A <cond> that holds no condition never bars the item.
		if !null {
			useConditions = append(useConditions, uc)
		}
	}

	return modifiers, useConditions, nil
}

// buildItemFunc builds one stat func of an item, directly under <for> or
// inside an <effect>. Its val reads the item's own tables; its stat is read
// as written, and its condition may name no table.
func buildItemFunc(id int32, op item.FuncOp, attrs []xml.Attr, children []condNode, tables map[string][]string) (item.StatModifier, error) {
	vals := foldAttrs(attrs)
	if raw, ok := vals["val"]; ok {
		resolved, err := resolveTableValue(tables, "val", raw, 1)
		if err != nil {
			return item.StatModifier{}, fmt.Errorf("item template %d: %w", id, err)
		}
		vals["val"] = resolved
	}
	mod, err := buildStatModifier(op, vals)
	if err != nil {
		return item.StatModifier{}, fmt.Errorf("item template %d: %w", id, err)
	}
	if len(children) > 0 {
		cond, err := buildCondition(children[0], condRolePredicate)
		if err != nil {
			return item.StatModifier{}, fmt.Errorf("item template %d: %s: %w", id, op, err)
		}
		mod.Condition = &cond
	}
	return mod, nil
}

// validateItemEffect reads a <for> block's <effect> child with the skill
// effect grammar and discards it: an item has no effect templates, but a
// malformed effect still rejects the item. The effect's val and abnormal and
// its funcs' vals read the item's own tables; every other value the effect
// reader would resolve against a template's tables, its own or its <cond>
// and func conditions', fails instead, because neither an item nor the
// effect is such a template. stackType and a func's stat are read as
// written.
func validateItemEffect(id int32, opEl funcElement, tables map[string][]string) error {
	vals := foldAttrs(opEl.Attrs)
	for _, name := range itemEffectTableAttrs {
		if err := noTableRef(name, vals[name]); err != nil {
			return fmt.Errorf("item template %d: effect: %w", id, err)
		}
	}
	for _, name := range []string{"val", "abnormal"} {
		raw, ok := vals[name]
		if !ok {
			continue
		}
		resolved, err := resolveTableValue(tables, name, raw, 1)
		if err != nil {
			return fmt.Errorf("item template %d: effect: %w", id, err)
		}
		vals[name] = resolved
	}
	if _, err := readEffectTemplate(vals); err != nil {
		return fmt.Errorf("item template %d: %w", id, err)
	}
	for i, ch := range opEl.Children {
		tag := ch.XMLName.Local
		switch {
		case strings.EqualFold(tag, "cond"):
			// Only a <cond> that is the effect's first node is read.
			if !leadsWithCond(tag, i, opEl.LeadingNode) {
				continue
			}
			if _, _, err := buildUseCondition(id, ch.Attrs, ch.Children); err != nil {
				return err
			}
		case strings.EqualFold(tag, "effect"):
			return fmt.Errorf("item template %d: effect: %w", id, errNestedEffect)
		default:
			op, err := item.ParseFuncOp(tag)
			if err != nil {
				// Unrecognized effect children are silently ignored.
				continue
			}
			if _, err := buildItemFunc(id, op, ch.Attrs, ch.Children, tables); err != nil {
				return err
			}
		}
	}
	return nil
}

// itemEffectTableAttrs are the <effect> attributes whose value the effect
// reader resolves against the template's tables.
var itemEffectTableAttrs = []string{
	"name", "count", "time", "self", "noicon", "stackOrder", "effectPower",
	"effectType", "triggeredId", "triggeredLevel", "chanceType", "activationChance",
}

// buildStatModifier reads one stat-modifier element's "stat" and "val"
// values (both required) from vals. The stat must name a known stat.
func buildStatModifier(op item.FuncOp, vals map[string]string) (item.StatModifier, error) {
	a := newAttrValues(vals, "stat modifier")
	mod := item.StatModifier{
		Op:    op,
		Stat:  readFuncStat(a),
		Value: a.float64("val"),
	}
	if err := a.Err(); err != nil {
		return item.StatModifier{}, err
	}
	return mod, nil
}

// buildUseCondition reads a <cond> element. An item has no tables, so
// neither its msgId nor any condition value it reads may name one. A <cond>
// with no predicate, or whose predicate holds no condition
// (conditions.IsNull), reports null and reads none of msg, msgId and
// addName.
func buildUseCondition(id int32, attrs []xml.Attr, children []condNode) (uc item.UseCondition, null bool, err error) {
	if len(children) == 0 {
		return item.UseCondition{}, true, nil
	}
	root, err := buildCondition(children[0], condRolePredicate)
	if err != nil {
		return item.UseCondition{}, false, fmt.Errorf("item template %d: cond: %w", id, err)
	}
	if conditions.IsNull(skillCondition(root)) {
		return item.UseCondition{}, true, nil
	}
	a := newAttrValues(foldAttrs(attrs), fmt.Sprintf("item template %d: use condition", id))

	switch {
	case a.has("msg"):
		uc.Message = a.strDefault("msg", "")
	case a.has("msgId"):
		if err := noTableRef("msgId", a.str("msgId")); err != nil {
			return item.UseCondition{}, false, fmt.Errorf("item template %d: cond: %w", id, err)
		}
		uc.MessageID = a.int32LiteralDefault("msgId", 0)
		if a.has("addName") && uc.MessageID > 0 {
			uc.AddName = true
		}
	}
	if err := a.Err(); err != nil {
		return item.UseCondition{}, false, err
	}
	uc.Root = root
	return uc, false, nil
}

// skillCondition is c in the skill condition shape conditions.Compile reads.
func skillCondition(c item.Condition) skill.Condition {
	out := skill.Condition{Kind: c.Kind, Attrs: c.Attrs}
	for _, ch := range c.Children {
		out.Children = append(out.Children, skillCondition(ch))
	}
	return out
}

// buildCondition converts one decoded condition node in role into an
// item.Condition, recursively converting its children. Values are kept as
// written; one the condition reader would take as a table reference fails.
func buildCondition(n condNode, role condRole) (item.Condition, error) {
	attrs, err := conditionAttrs(n, role, nil)
	if err != nil {
		return item.Condition{}, fmt.Errorf("<%s>: %w", n.XMLName.Local, err)
	}
	var children []item.Condition
	for i, c := range n.Children {
		child, err := buildCondition(c, childConditionRole(n, role, i))
		if err != nil {
			return item.Condition{}, err
		}
		children = append(children, child)
	}
	return item.Condition{
		Kind:     strings.ToLower(n.XMLName.Local),
		Attrs:    attrs,
		Children: children,
	}, nil
}
