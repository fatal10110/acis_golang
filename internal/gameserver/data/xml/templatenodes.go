package xml

import (
	"encoding/xml"
	"strings"
)

// A <for>, <enchantNfor> or <effect> block reads a <cond> as its attach
// condition only when that <cond> is the block's very first node. Any
// character data (whitespace included) or processing instruction before it
// is a node of its own, so a pretty-printed leading <cond> is never read.
// Comments are not nodes and do not count. The standard decoder drops that
// character data, so these blocks decode their children by hand and record
// whether anything came before the first child element.

// UnmarshalXML decodes a <for> block's child elements, noting whether a
// non-element node precedes the first one.
func (f *forElement) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	leading, err := decodeChildElements(d, func(child xml.StartElement) error {
		var op funcElement
		if err := d.DecodeElement(&op, &child); err != nil {
			return err
		}
		f.Ops = append(f.Ops, op)
		return nil
	})
	f.LeadingNode = leading
	return err
}

// UnmarshalXML decodes one child of a <for> block: its tag, attributes and
// child elements, noting whether a non-element node precedes the first child
// element.
func (e *funcElement) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	e.XMLName = start.Name
	e.Attrs = start.Attr
	leading, err := decodeChildElements(d, func(child xml.StartElement) error {
		var n condNode
		if err := d.DecodeElement(&n, &child); err != nil {
			return err
		}
		e.Children = append(e.Children, n)
		return nil
	})
	e.LeadingNode = leading
	return err
}

// decodeChildElements consumes the current element's content up to its end
// tag, handing each child element to decode. It reports whether character
// data or a processing instruction came before the first child element.
func decodeChildElements(d *xml.Decoder, decode func(xml.StartElement) error) (leadingNode bool, err error) {
	seenElement := false
	for {
		tok, err := d.Token()
		if err != nil {
			return leadingNode, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			seenElement = true
			if err := decode(t); err != nil {
				return leadingNode, err
			}
		case xml.CharData, xml.ProcInst:
			if !seenElement {
				leadingNode = true
			}
		case xml.EndElement:
			return leadingNode, nil
		}
	}
}

// leadsWithCond reports whether the child at index i of a block is its
// attach condition: a <cond> that is the block's first node.
func leadsWithCond(tag string, i int, leadingNode bool) bool {
	return i == 0 && !leadingNode && strings.EqualFold(tag, "cond")
}
