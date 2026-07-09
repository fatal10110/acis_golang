package xml

import (
	"encoding/xml"
	"fmt"
	"strings"

	"github.com/fatal10110/acis_golang/internal/commons"
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

// buildItemTemplate packs one parsed <item> element into the StatSet shape
// item.NewTemplate consumes: its own attributes and <set> children merged
// flat, plus the "modifiers" and "useConditions" values built from its
// <for> and <cond> children.
func buildItemTemplate(el itemElement) (*item.Template, error) {
	set := commons.StatSetFromXMLAttrs(el.Attrs)
	tables, err := buildValueTables(el.Tables)
	if err != nil {
		return nil, err
	}
	for _, s := range el.Sets {
		val, err := resolveTableValue(tables, s.Name, s.Val, 1)
		if err != nil {
			return nil, err
		}
		set.Set(s.Name, val)
	}
	id := set.GetStringDefault("id", "?")

	var modifiers []item.StatModifier
	for _, forEl := range el.For {
		var attachCond *item.UseCondition
		for _, opEl := range forEl.Ops {
			if strings.EqualFold(opEl.XMLName.Local, "cond") {
				uc, err := buildUseCondition(id, opEl.Attrs, opEl.Children)
				if err != nil {
					return nil, err
				}
				attachCond = &uc
				continue
			}

			op, err := item.ParseFuncOp(opEl.XMLName.Local)
			if err != nil {
				return nil, fmt.Errorf("item %s: %w", id, err)
			}
			attrs, err := resolveModifierAttrs(tables, opEl.Attrs)
			if err != nil {
				return nil, fmt.Errorf("item %s: %w", id, err)
			}
			mod, err := item.NewStatModifier(op, commons.StatSetFromXMLAttrs(attrs))
			if err != nil {
				return nil, fmt.Errorf("item %s: %w", id, err)
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
	if modifiers != nil {
		set.Set("modifiers", modifiers)
	}

	var useConditions []item.UseCondition
	for _, condEl := range el.Cond {
		uc, err := buildUseCondition(id, condEl.Attrs, condEl.Children)
		if err != nil {
			return nil, err
		}
		useConditions = append(useConditions, uc)
	}
	if useConditions != nil {
		set.Set("useConditions", useConditions)
	}

	return item.NewTemplate(set)
}

func resolveModifierAttrs(tables map[string][]string, attrs []xml.Attr) ([]xml.Attr, error) {
	resolved := attrs
	for i, a := range attrs {
		if a.Name.Local != "val" {
			continue
		}
		val, err := resolveTableValue(tables, a.Name.Local, a.Value, 1)
		if err != nil {
			return nil, err
		}
		if val != a.Value {
			resolved = append([]xml.Attr(nil), attrs...)
			resolved[i].Value = val
		}
	}
	return resolved, nil
}

func buildUseCondition(id string, attrs []xml.Attr, children []condNode) (item.UseCondition, error) {
	if len(children) == 0 {
		return item.UseCondition{}, fmt.Errorf("item %s: cond: no predicate defined", id)
	}
	root := buildCondition(children[0])
	uc, err := item.NewUseCondition(root, commons.StatSetFromXMLAttrs(attrs))
	if err != nil {
		return item.UseCondition{}, fmt.Errorf("item %s: %w", id, err)
	}
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
