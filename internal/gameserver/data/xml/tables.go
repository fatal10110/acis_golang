package xml

import (
	"encoding/xml"
	"errors"
	"fmt"
	"strings"
)

// tableElement is one <table> element: a named, whitespace-separated row of
// substitution values referenced elsewhere by "#name". It decodes itself.
type tableElement struct {
	Name string
	// Text is the value of the element's first child node, the only one
	// the row is read from: a run of character data, or the data of a
	// processing instruction. HasText is false when there is no such node:
	// the element is empty, holds only comments, or starts with an element.
	Text    string
	HasText bool
}

// errTableNoText marks a <table> whose first child node holds no value.
var errTableNoText = errors.New("first child node holds no text")

// UnmarshalXML decodes a <table> element, keeping only its first child
// node's value. Comments are not nodes, and the character data around one
// joins into a single node.
func (t *tableElement) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	for _, a := range start.Attr {
		if a.Name.Local == "name" {
			t.Name = a.Value
		}
	}
	var text strings.Builder
	err := decodeChildNodes(d, func(node int, tok xml.Token) error {
		switch tk := tok.(type) {
		case xml.StartElement:
			return d.Skip()
		case xml.CharData:
			if node == 0 {
				text.Write(tk)
				t.HasText = true
			}
		case xml.ProcInst:
			if node == 0 {
				text.Write(tk.Inst)
				t.HasText = true
			}
		}
		return nil
	})
	t.Text = text.String()
	return err
}

func buildValueTables(elems []tableElement) (map[string][]string, error) {
	tables := make(map[string][]string, len(elems))
	for _, tbl := range elems {
		if !strings.HasPrefix(tbl.Name, "#") {
			return nil, fmt.Errorf("table name %q must start with '#'", tbl.Name)
		}
		if !tbl.HasText {
			return nil, fmt.Errorf("table %q: %w", tbl.Name, errTableNoText)
		}
		tables[tbl.Name] = strings.FieldsFunc(tbl.Text, isTableSpace)
	}
	return tables, nil
}

// isTableSpace reports whether r separates two values of a table row: a
// space, tab, line feed, carriage return or form feed. Other white space,
// such as a vertical tab or a no-break space, is part of a value.
func isTableSpace(r rune) bool {
	switch r {
	case ' ', '\t', '\n', '\r', '\f':
		return true
	}
	return false
}

func resolveTableValue(tables map[string][]string, name, val string, tableIndex int) (string, error) {
	if !strings.HasPrefix(val, "#") {
		return val, nil
	}
	row, ok := tables[val]
	if !ok {
		return "", fmt.Errorf("attribute %q references undefined table %q", name, val)
	}
	if tableIndex < 1 || tableIndex > len(row) {
		return "", fmt.Errorf("attribute %q: table %q has no row %d (has %d)", name, val, tableIndex, len(row))
	}
	return row[tableIndex-1], nil
}
