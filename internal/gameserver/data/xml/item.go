package xml

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
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
// fold in directly; <set> children flatten alongside them; <for> and <cond>
// are distinctly shaped child blocks handled by their own types. A <table>
// child (a named, whitespace-tokenized value list referenced elsewhere as
// "#name") is left undeclared and skipped by the decoder: no shipped item
// file defines one.
type itemElement struct {
	Attrs []xml.Attr    `xml:",any,attr"`
	Sets  []setElem     `xml:"set"`
	For   []forElement  `xml:"for"`
	Cond  []condElement `xml:"cond"`
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
	XMLName xml.Name
	Attrs   []xml.Attr `xml:",any,attr"`
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
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("data/xml: read item template dir %s: %w", dir, err)
	}

	var templates []*item.Template
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".xml") {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		parsed, err := parseItemFile(path)
		if err != nil {
			return nil, err
		}

		for _, el := range parsed.Items {
			tpl, err := buildItemTemplate(el)
			if err != nil {
				return nil, fmt.Errorf("data/xml: parse item in %s: %w", path, err)
			}
			templates = append(templates, tpl)
		}
	}

	return item.NewTable(templates), nil
}

// parseItemFile decodes one item template XML file.
func parseItemFile(path string) (*itemFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("data/xml: read %s: %w", path, err)
	}

	var parsed itemFile
	if err := xml.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("data/xml: parse %s: %w", path, err)
	}
	return &parsed, nil
}

// buildItemTemplate packs one parsed <item> element into the StatSet shape
// item.NewTemplate consumes: its own attributes and <set> children merged
// flat, plus the "modifiers" and "useConditions" values built from its
// <for> and <cond> children.
func buildItemTemplate(el itemElement) (*item.Template, error) {
	set := commons.StatSetFromXMLAttrs(el.Attrs)
	for _, s := range el.Sets {
		set.Set(s.Name, s.Val)
	}
	id := set.GetStringDefault("id", "?")

	var modifiers []item.StatModifier
	for _, forEl := range el.For {
		for _, opEl := range forEl.Ops {
			op, err := item.ParseFuncOp(opEl.XMLName.Local)
			if err != nil {
				return nil, fmt.Errorf("item %s: %w", id, err)
			}
			mod, err := item.NewStatModifier(op, commons.StatSetFromXMLAttrs(opEl.Attrs))
			if err != nil {
				return nil, fmt.Errorf("item %s: %w", id, err)
			}
			modifiers = append(modifiers, mod)
		}
	}
	if modifiers != nil {
		set.Set("modifiers", modifiers)
	}

	var useConditions []item.UseCondition
	for _, condEl := range el.Cond {
		if len(condEl.Children) == 0 {
			return nil, fmt.Errorf("item %s: cond: no predicate defined", id)
		}
		// Only the first child predicate is meaningful: a <cond> block
		// wraps exactly one root expression (a leaf predicate or a single
		// and/or/not combinator), the same shape every shipped file uses.
		root := buildCondition(condEl.Children[0])
		uc, err := item.NewUseCondition(root, commons.StatSetFromXMLAttrs(condEl.Attrs))
		if err != nil {
			return nil, fmt.Errorf("item %s: %w", id, err)
		}
		useConditions = append(useConditions, uc)
	}
	if useConditions != nil {
		set.Set("useConditions", useConditions)
	}

	return item.NewTemplate(set)
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
