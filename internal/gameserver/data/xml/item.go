package xml

import (
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
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
// since they share one attribute shape and differ only by tag name.
type forElement struct {
	Ops []funcElement `xml:",any"`
}

// funcElement is one stat-modifier element inside a <for> block; XMLName
// carries which operation it applies (see item.ParseFuncOp).
type funcElement struct {
	XMLName  xml.Name
	Attrs    []xml.Attr `xml:",any,attr"`
	Children []condNode `xml:",any"`
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
// A directory that can't be listed, a file whose XML is not well-formed, or
// an individual <item> that can't be turned into a Template fails the whole
// load: the caller gets an actionable error rather than a partially
// populated table.
func LoadItemTemplates(dir string) (*item.Table, error) {
	docs, err := loadXMLDocuments[itemFile](dir, "item template")
	if err != nil {
		return nil, err
	}

	var templates []*item.Template
	for _, doc := range docs {
		for _, el := range doc.Data.Items {
			tpl, err := buildItemTemplate(el)
			if err != nil {
				return nil, fmt.Errorf("data/xml: parse item in %s: %w", doc.Path, err)
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
	rate, err := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 32)
	if err != nil {
		a.fail(fmt.Errorf("attribute %q: %w", key, err))
		return 0, 0
	}
	value, err := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 32)
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
		ItemID: a.int32("itemid"),
		Min:    a.int32("min"),
		Max:    a.int32("max"),
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
		for _, opEl := range forEl.Ops {
			if strings.EqualFold(opEl.XMLName.Local, "cond") {
				uc, err := buildUseCondition(id, opEl.Attrs, opEl.Children)
				if err != nil {
					return nil, nil, err
				}
				attachCond = &uc
				continue
			}

			op, err := item.ParseFuncOp(opEl.XMLName.Local)
			if err != nil {
				return nil, nil, fmt.Errorf("item template %d: %w", id, err)
			}
			vals := foldAttrs(opEl.Attrs)
			if raw, ok := vals["val"]; ok {
				resolved, err := resolveTableValue(tables, "val", raw, 1)
				if err != nil {
					return nil, nil, fmt.Errorf("item template %d: %w", id, err)
				}
				vals["val"] = resolved
			}
			mod, err := buildStatModifier(op, vals)
			if err != nil {
				return nil, nil, fmt.Errorf("item template %d: %w", id, err)
			}
			if attachCond != nil {
				mod.AttachCondition = attachCond
			}
			if len(opEl.Children) > 0 {
				cond := buildCondition(opEl.Children[0])
				mod.Condition = &cond
			}
			modifiers = append(modifiers, mod)
		}
	}

	var useConditions []item.UseCondition
	for _, condEl := range el.Cond {
		uc, err := buildUseCondition(id, condEl.Attrs, condEl.Children)
		if err != nil {
			return nil, nil, err
		}
		useConditions = append(useConditions, uc)
	}

	return modifiers, useConditions, nil
}

// buildStatModifier reads one stat-modifier element's "stat" and "val"
// values (both required) from vals.
func buildStatModifier(op item.FuncOp, vals map[string]string) (item.StatModifier, error) {
	a := newAttrValues(vals, "stat modifier")
	mod := item.StatModifier{
		Op:    op,
		Stat:  a.str("stat"),
		Value: a.float64("val"),
	}
	if err := a.Err(); err != nil {
		return item.StatModifier{}, err
	}
	return mod, nil
}

func buildUseCondition(id int32, attrs []xml.Attr, children []condNode) (item.UseCondition, error) {
	if len(children) == 0 {
		return item.UseCondition{}, fmt.Errorf("item template %d: cond: no predicate defined", id)
	}
	root := buildCondition(children[0])
	a := newAttrValues(foldAttrs(attrs), fmt.Sprintf("item template %d: use condition", id))

	var uc item.UseCondition
	switch {
	case a.has("msg"):
		uc.Message = a.strDefault("msg", "")
	case a.has("msgId"):
		uc.MessageID = a.int32("msgId")
		if a.has("addName") && uc.MessageID > 0 {
			uc.AddName = true
		}
	}
	if err := a.Err(); err != nil {
		return item.UseCondition{}, err
	}
	uc.Root = root
	return uc, nil
}

// buildCondition converts one decoded condition node into an item.Condition,
// recursively converting its children.
func buildCondition(n condNode) item.Condition {
	attrs := make(map[string]string, len(n.Attrs))
	for _, a := range n.Attrs {
		attrs[a.Name.Local] = a.Value
	}
	var children []item.Condition
	for _, c := range n.Children {
		children = append(children, buildCondition(c))
	}
	return item.Condition{
		Kind:     strings.ToLower(n.XMLName.Local),
		Attrs:    attrs,
		Children: children,
	}
}
