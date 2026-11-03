package manager

import (
	"encoding/xml"
	"fmt"
	"os"
	"sort"
	"strconv"
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
		ID   *serverID `xml:"id,attr"`
		Name string    `xml:"name,attr"`
	} `xml:"server"`
}

// serverID is a required plain base-10 int32 id attribute. Empty, padded
// and non-numeric values fail the load, where the decoder's own int
// conversion would read an empty value as 0 and trim the padding.
type serverID int32

func (id *serverID) UnmarshalXMLAttr(attr xml.Attr) error {
	n, err := strconv.ParseInt(attr.Value, 10, 32)
	if err != nil {
		return fmt.Errorf("%s: %w", attr.Name.Local, err)
	}
	*id = serverID(n)
	return nil
}

// LoadServerNames reads the id/name list from path.
func LoadServerNames(path string) (*ServerNames, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read server names %s: %w", path, err)
	}

	var doc serverNamesFile
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse server names %s: %w", path, err)
	}

	n := &ServerNames{names: make(map[int]string, len(doc.Servers))}
	for _, s := range doc.Servers {
		if s.ID == nil {
			return nil, fmt.Errorf("parse server names %s: server %q: id is required", path, s.Name)
		}
		id := int(*s.ID)
		if _, seen := n.names[id]; !seen {
			n.ids = append(n.ids, id)
		}
		n.names[id] = s.Name // a repeated id keeps its last name
	}
	sort.Ints(n.ids)
	return n, nil
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
