package manager

import (
	"encoding/xml"
	"fmt"
	"os"
	"sort"
)

// ServerNames is the static id -> display-name table used to name a newly
// registered game server and to offer a free id when a game server's
// desired id is taken by a different auth key.
type ServerNames struct {
	names map[int]string
	ids   []int // sorted ascending
}

type serverNamesFile struct {
	Servers []struct {
		ID   int    `xml:"id,attr"`
		Name string `xml:"name,attr"`
	} `xml:"server"`
}

// LoadServerNames reads the id/name list from path.
func LoadServerNames(path string) (*ServerNames, error) {
	var doc serverNamesFile
	if err := readXML(path, &doc); err != nil {
		return nil, fmt.Errorf("server names: %w", err)
	}

	n := &ServerNames{names: make(map[int]string, len(doc.Servers))}
	for _, s := range doc.Servers {
		n.names[s.ID] = s.Name
		n.ids = append(n.ids, s.ID)
	}
	sort.Ints(n.ids)
	return n, nil
}

// readXML reads and unmarshals the XML document at path into dst, wrapping
// either failure with the path and operation.
func readXML(path string, dst any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("xml: read %s: %w", path, err)
	}
	if err := xml.Unmarshal(data, dst); err != nil {
		return fmt.Errorf("xml: parse %s: %w", path, err)
	}
	return nil
}

// Name returns the display name registered for id.
func (n *ServerNames) Name(id int) (string, bool) {
	name, ok := n.names[id]
	return name, ok
}

// IDs returns every known id in ascending order.
func (n *ServerNames) IDs() []int {
	return append([]int(nil), n.ids...)
}
