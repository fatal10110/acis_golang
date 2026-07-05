package xml

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// itemFile is the root element of one item template XML file: a flat list
// of <item> elements, each optionally carrying <set name="..." val="..."/>
// attribute children. Only the elements and attributes this loader reads
// are declared; every other element a shipped file may contain (stat bonus
// blocks, use conditions, per-level tables) is left undeclared and is
// therefore skipped by the decoder rather than rejected.
type itemFile struct {
	Items []itemElement `xml:"item"`
}

type itemElement struct {
	ID   string    `xml:"id,attr"`
	Type string    `xml:"type,attr"`
	Name string    `xml:"name,attr"`
	Sets []setElem `xml:"set"`
}

type setElem struct {
	Name string `xml:"name,attr"`
	Val  string `xml:"val,attr"`
}

// LoadItemTemplates parses every ".xml" item template file directly under
// dir and returns a lookup table of the resulting templates keyed by item
// id. dir is expected to look like a shipped aCis_datapack
// "data/xml/items" directory: one flat list of files, each holding a flat
// list of <item> elements.
//
// Only the fields character creation and world entry need to grant and
// equip starter gear are extracted (id, name, kind, equip slot,
// stackability, per the Template documentation) — combat stat bonuses, use
// conditions, and skill/effect wiring present in the shipped files are not
// modeled.
//
// A directory that can't be listed, or a file whose XML is not
// well-formed, fails the whole load: the caller gets an error rather than a
// partially populated table. An individual <item> element that can't be
// turned into a Template (an unresolvable id, kind, or equip slot) is
// logged and skipped, so one bad entry doesn't take down every other
// template in the same file.
//
// log receives skipped-item diagnostics; a nil log defaults to
// logrus.StandardLogger().
func LoadItemTemplates(dir string, log *logrus.Logger) (*item.Table, error) {
	if log == nil {
		log = logrus.StandardLogger()
	}

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
				log.Warnf("data/xml: skipping item in %s: %v", path, err)
				continue
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

// buildItemTemplate turns one decoded <item> element into a Template,
// reading only the <set> attributes the minimal Template needs.
func buildItemTemplate(el itemElement) (*item.Template, error) {
	id, err := strconv.ParseInt(el.ID, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("id %q: %w", el.ID, err)
	}

	kind, err := item.ParseKind(el.Type)
	if err != nil {
		return nil, fmt.Errorf("item %d: %w", id, err)
	}

	sets := make(map[string]string, len(el.Sets))
	for _, s := range el.Sets {
		sets[s.Name] = s.Val
	}

	bodyPart, ok := sets["bodypart"]
	if !ok {
		bodyPart = "none"
	}
	slot, err := item.ParseSlot(bodyPart)
	if err != nil {
		return nil, fmt.Errorf("item %d: %w", id, err)
	}

	return &item.Template{
		ID:        int32(id),
		Name:      el.Name,
		Kind:      kind,
		Slot:      slot,
		Stackable: sets["is_stackable"] == "true",
	}, nil
}
