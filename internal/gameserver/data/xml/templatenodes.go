package xml

import (
	"bytes"
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
//
// A CDATA section is a node of its own, even when empty: it never joins the
// text before or after it, nor another CDATA section. The standard decoder
// reports a CDATA section as plain character data, so skill and item files
// are decoded through cdataMarker, which brackets each one with cdataBound
// tokens.

// cdataBound is the token cdataMarker emits right before and right after the
// character data of a CDATA section. Decoders that do not look for it skip
// it like any token they do not handle.
type cdataBound struct{}

// cdataMarker is the token stream of a document held in data, with every
// CDATA section bracketed by cdataBound tokens.
type cdataMarker struct {
	data    []byte
	raw     *xml.Decoder
	pending []xml.Token
}

// newTemplateDecoder returns a decoder over data that reports CDATA section
// bounds (see cdataMarker).
func newTemplateDecoder(data []byte) *xml.Decoder {
	return xml.NewTokenDecoder(&cdataMarker{data: data, raw: xml.NewDecoder(bytes.NewReader(data))})
}

// Token returns the next token of the document. The returned decoder checks
// element nesting and resolves names, so the raw tokens are passed on as
// read. Character data whose first input byte is '<' can only be a CDATA
// section: text never starts with one.
func (m *cdataMarker) Token() (xml.Token, error) {
	if len(m.pending) > 0 {
		tok := m.pending[0]
		m.pending = m.pending[1:]
		return tok, nil
	}
	start := m.raw.InputOffset()
	tok, err := m.raw.RawToken()
	if err != nil {
		return tok, err
	}
	if text, ok := tok.(xml.CharData); ok && start < int64(len(m.data)) && m.data[start] == '<' {
		m.pending = append(m.pending, text.Copy(), cdataBound{})
		return cdataBound{}, nil
	}
	return tok, nil
}

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
	err = decodeChildNodes(d, func(node int, tok xml.Token) error {
		child, ok := tok.(xml.StartElement)
		if !ok {
			return nil
		}
		if !seenElement {
			seenElement = true
			leadingNode = node > 0
		}
		return decode(child)
	})
	return leadingNode, err
}

// decodeChildNodes consumes the current element's content up to its end tag,
// handing visit each token with the index of the child node it belongs to. A
// run of character data is one node, even when comments split it, since
// comments are not nodes; a CDATA section, a processing instruction and an
// element are one node each. visit must consume a child element it is
// handed, by decoding or skipping it.
func decodeChildNodes(d *xml.Decoder, visit func(node int, tok xml.Token) error) error {
	node := -1
	inText := false
	for {
		tok, err := d.Token()
		if err != nil {
			return err
		}
		switch tok.(type) {
		case xml.CharData:
			if !inText {
				node++
				inText = true
			}
		case xml.StartElement, xml.ProcInst:
			node++
			inText = false
		case cdataBound:
			inText = false
			continue
		case xml.EndElement:
			return nil
		default:
			continue
		}
		if err := visit(node, tok); err != nil {
			return err
		}
	}
}

// leadsWithCond reports whether the child at index i of a block is its
// attach condition: a <cond> that is the block's first node.
func leadsWithCond(tag string, i int, leadingNode bool) bool {
	return i == 0 && !leadingNode && strings.EqualFold(tag, "cond")
}
