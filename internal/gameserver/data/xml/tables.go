package xml

import (
	"fmt"
	"strings"
)

// tableElement is one <table> element: a named, whitespace-separated row of
// substitution values referenced elsewhere by "#name".
type tableElement struct {
	Name string `xml:"name,attr"`
	Text string `xml:",chardata"`
}

func buildValueTables(elems []tableElement) (map[string][]string, error) {
	tables := make(map[string][]string, len(elems))
	for _, tbl := range elems {
		if !strings.HasPrefix(tbl.Name, "#") {
			return nil, fmt.Errorf("table name %q must start with '#'", tbl.Name)
		}
		tables[tbl.Name] = strings.Fields(tbl.Text)
	}
	return tables, nil
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
