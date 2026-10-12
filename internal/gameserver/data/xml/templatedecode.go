package xml

import (
	"encoding/xml"
	"errors"
	"strings"
)

// Skill and item files match every element tag the reader looks for
// ignoring case (<List>, <Skill>, <SET>, <Enchant1For>, ...), while attribute
// names match exactly. A file's root must be a <list>; a skill file may
// instead be a single root <skill>. Any other root loads nothing.

// UnmarshalXML decodes a skill file's root element.
func (f *skillFile) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	switch {
	case tagIs(start, "list"):
		_, err := decodeChildElements(d, func(child xml.StartElement) error {
			if tagIs(child, "skill") {
				return decodeAppend(d, child, &f.Skills)
			}
			return d.Skip()
		})
		return err
	case tagIs(start, "skill"):
		return decodeAppend(d, start, &f.Skills)
	}
	return d.Skip()
}

// UnmarshalXML decodes one <skill> element.
func (el *skillElement) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	for _, a := range start.Attr {
		v := a.Value
		switch a.Name.Local {
		case "id":
			el.ID = v
		case "name":
			el.Name = &v
		case "levels":
			el.Levels = v
		case "enchantLevels1":
			el.EnchantLevels1 = &v
		case "enchantLevels2":
			el.EnchantLevels2 = &v
		}
	}
	_, err := decodeChildElements(d, func(child xml.StartElement) error {
		switch {
		case tagIs(child, "table"):
			return decodeAppend(d, child, &el.Tables)
		case tagIs(child, "set"):
			return decodeAppend(d, child, &el.Sets)
		case tagIs(child, "enchant1"):
			return decodeAppend(d, child, &el.Enchant1)
		case tagIs(child, "enchant2"):
			return decodeAppend(d, child, &el.Enchant2)
		case tagIs(child, "cond"):
			return decodeAppend(d, child, &el.Cond)
		case tagIs(child, "for"):
			return decodeAppend(d, child, &el.For)
		case tagIs(child, "enchant1cond"):
			return decodeAppend(d, child, &el.Enchant1Cond)
		case tagIs(child, "enchant1for"):
			return decodeAppend(d, child, &el.Enchant1For)
		case tagIs(child, "enchant2cond"):
			return decodeAppend(d, child, &el.Enchant2Cond)
		case tagIs(child, "enchant2for"):
			return decodeAppend(d, child, &el.Enchant2For)
		}
		return d.Skip()
	})
	return err
}

// UnmarshalXML decodes an item file's root element.
func (f *itemFile) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	if !tagIs(start, "list") {
		return d.Skip()
	}
	_, err := decodeChildElements(d, func(child xml.StartElement) error {
		if tagIs(child, "item") {
			return decodeAppend(d, child, &f.Items)
		}
		return d.Skip()
	})
	return err
}

// UnmarshalXML decodes one <item> element.
func (el *itemElement) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	el.Attrs = start.Attr
	_, err := decodeChildElements(d, func(child xml.StartElement) error {
		switch {
		case tagIs(child, "table"):
			return decodeAppend(d, child, &el.Tables)
		case tagIs(child, "set"):
			return decodeAppend(d, child, &el.Sets)
		case tagIs(child, "for"):
			return decodeAppend(d, child, &el.For)
		case tagIs(child, "cond"):
			return decodeAppend(d, child, &el.Cond)
		}
		return d.Skip()
	})
	return err
}

// UnmarshalXML decodes one condition tree node, recording each child
// element's position among its child nodes.
func (n *condNode) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	n.XMLName, n.Attrs = start.Name, start.Attr
	return decodeChildNodes(d, func(node int, tok xml.Token) error {
		child, ok := tok.(xml.StartElement)
		if !ok {
			return nil
		}
		var c condNode
		if err := d.DecodeElement(&c, &child); err != nil {
			return err
		}
		c.Pos = node
		n.Children = append(n.Children, c)
		return nil
	})
}

// tagIs reports whether el's tag is tag, ignoring case.
func tagIs(el xml.StartElement, tag string) bool {
	return strings.EqualFold(el.Name.Local, tag)
}

// decodeAppend decodes the element start opens and appends it to dst.
func decodeAppend[T any](d *xml.Decoder, start xml.StartElement, dst *[]T) error {
	var v T
	if err := d.DecodeElement(&v, &start); err != nil {
		return err
	}
	*dst = append(*dst, v)
	return nil
}

// templateSet is one <set name val/> of a skill or an item, or one
// <enchant1>/<enchant2> of a skill. A missing attribute is nil.
type templateSet struct {
	Name *string `xml:"name,attr"`
	Val  *string `xml:"val,attr"`
}

// setValue is a read templateSet.
type setValue struct {
	name, val string
}

// readTemplateSets reads sets in order: each needs both its name and its
// val, and both are trimmed of leading and trailing spaces and control
// characters.
func readTemplateSets(sets []templateSet) ([]setValue, error) {
	out := make([]setValue, 0, len(sets))
	for _, s := range sets {
		if s.Name == nil {
			return nil, errSetNoName
		}
		if s.Val == nil {
			return nil, errSetNoVal
		}
		out = append(out, setValue{name: trimControlSpace(*s.Name), val: trimControlSpace(*s.Val)})
	}
	return out, nil
}

var (
	errSetNoName = errors.New(`<set>: attribute "name" is missing`)
	errSetNoVal  = errors.New(`<set>: attribute "val" is missing`)
)

// trimControlSpace trims every leading and trailing character at or below
// U+0020: spaces and control characters, not other white space.
func trimControlSpace(s string) string {
	return strings.TrimFunc(s, func(r rune) bool { return r <= ' ' })
}
