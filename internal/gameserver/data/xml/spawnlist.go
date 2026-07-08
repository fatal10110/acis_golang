package xml

import (
	"encoding/xml"
	"fmt"
	"strings"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
)

type spawnlistFile struct {
	Territories []territoryElement `xml:"territory"`
	Makers      []makerElement     `xml:"npcmaker"`
}

type territoryElement struct {
	Attrs []xml.Attr        `xml:",any,attr"`
	Nodes []territoryNodeEl `xml:"node"`
}

type territoryNodeEl struct {
	Attrs []xml.Attr `xml:",any,attr"`
}

type makerElement struct {
	Attrs []xml.Attr        `xml:",any,attr"`
	AI    []aiElement       `xml:"ai"`
	NPCs  []spawnNPCElement `xml:"npc"`
}

type aiElement struct {
	Attrs []xml.Attr `xml:",any,attr"`
	Sets  []setElem  `xml:"set"`
}

type spawnNPCElement struct {
	Attrs    []xml.Attr        `xml:",any,attr"`
	Privates []spawnPrivatesEl `xml:"privates"`
	AI       []aiElement       `xml:"ai"`
}

type spawnPrivatesEl struct {
	Entries []spawnPrivateEl `xml:"private"`
}

type spawnPrivateEl struct {
	Attrs []xml.Attr `xml:",any,attr"`
}

// LoadSpawnlist parses every region-sharded spawnlist XML file directly under
// dir and returns the full in-memory territory/maker table.
func LoadSpawnlist(dir string) (*spawn.Table, error) {
	docs, err := loadXMLDocuments[spawnlistFile](dir, "spawnlist")
	if err != nil {
		return nil, err
	}

	territoryMap := make(map[string]*spawn.Territory)
	var territories []*spawn.Territory
	var makers []*spawn.Maker

	for _, doc := range docs {
		for _, el := range doc.Data.Territories {
			territory, err := buildTerritory(el)
			if err != nil {
				return nil, fmt.Errorf("data/xml: parse territory in %s: %w", doc.Path, err)
			}
			if existing, exists := territoryMap[territory.Name]; exists {
				if !sameSpawnTerritory(existing, territory) {
					return nil, fmt.Errorf("data/xml: parse territory in %s: conflicting duplicate territory %q", doc.Path, territory.Name)
				}
			} else {
				territoryMap[territory.Name] = territory
			}
			territories = append(territories, territory)
		}
	}

	for _, doc := range docs {
		for _, el := range doc.Data.Makers {
			maker, err := buildMaker(el, territoryMap)
			if err != nil {
				return nil, fmt.Errorf("data/xml: parse maker in %s: %w", doc.Path, err)
			}
			makers = append(makers, maker)
		}
	}

	return spawn.NewTable(territories, makers)
}

func buildTerritory(el territoryElement) (*spawn.Territory, error) {
	nodes := make([]spawn.Node, 0, len(el.Nodes))
	for _, nodeEl := range el.Nodes {
		set := commons.StatSetFromXMLAttrs(nodeEl.Attrs)
		x, err := set.GetInt("x")
		if err != nil {
			return nil, err
		}
		y, err := set.GetInt("y")
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, spawn.Node{X: x, Y: y})
	}
	return spawn.NewTerritory(commons.StatSetFromXMLAttrs(el.Attrs), nodes)
}

func sameSpawnTerritory(a, b *spawn.Territory) bool {
	if a.Name != b.Name || a.MinZ != b.MinZ || a.MaxZ != b.MaxZ || len(a.Nodes) != len(b.Nodes) {
		return false
	}
	for i, node := range a.Nodes {
		if node != b.Nodes[i] {
			return false
		}
	}
	return true
}

func buildMaker(el makerElement, territories map[string]*spawn.Territory) (*spawn.Maker, error) {
	set := commons.StatSetFromXMLAttrs(el.Attrs)

	refs, err := resolveTerritories(set.GetStringDefault("territory", ""), territories)
	if err != nil {
		return nil, fmt.Errorf("maker %q: %w", set.GetStringDefault("name", "?"), err)
	}
	banned, err := resolveTerritories(set.GetStringDefault("ban", ""), territories)
	if err != nil {
		return nil, fmt.Errorf("maker %q: %w", set.GetStringDefault("name", "?"), err)
	}

	aiType, aiParams := flattenAI(el.AI)
	if aiType != "" {
		set.Set("maker", aiType)
	}

	entries := make([]spawn.Entry, 0, len(el.NPCs))
	for _, npcEl := range el.NPCs {
		entry, err := buildEntry(npcEl)
		if err != nil {
			return nil, fmt.Errorf("maker %q: %w", set.GetStringDefault("name", "?"), err)
		}
		entries = append(entries, entry)
	}

	return spawn.NewMaker(set, refs, banned, entries, aiParams)
}

func buildEntry(el spawnNPCElement) (spawn.Entry, error) {
	set := commons.StatSetFromXMLAttrs(el.Attrs)

	privates := make([]spawn.Private, 0)
	for _, group := range el.Privates {
		for _, privateEl := range group.Entries {
			privateSpawn, err := spawn.NewPrivate(commons.StatSetFromXMLAttrs(privateEl.Attrs))
			if err != nil {
				return spawn.Entry{}, fmt.Errorf("npc %q private: %w", set.GetStringDefault("id", "?"), err)
			}
			privates = append(privates, privateSpawn)
		}
	}

	_, aiParams := flattenAI(el.AI)
	positions, err := spawn.ParsePositions(set.GetStringDefault("pos", ""))
	if err != nil {
		return spawn.Entry{}, fmt.Errorf("npc %q: %w", set.GetStringDefault("id", "?"), err)
	}

	entry, err := spawn.NewEntry(set, positions, privates, aiParams)
	if err != nil {
		return spawn.Entry{}, fmt.Errorf("npc %q: %w", set.GetStringDefault("id", "?"), err)
	}
	return entry, nil
}

func flattenAI(ai []aiElement) (string, map[string]string) {
	var kind string
	params := make(map[string]string)
	for _, el := range ai {
		set := commons.StatSetFromXMLAttrs(el.Attrs)
		kind = set.GetStringDefault("type", kind)
		for _, param := range el.Sets {
			params[param.Name] = strings.ReplaceAll(param.Val, "@", "")
		}
	}
	if len(params) == 0 {
		params = nil
	}
	return kind, params
}

func resolveTerritories(raw string, territories map[string]*spawn.Territory) ([]*spawn.Territory, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	names := strings.Split(raw, ";")
	resolved := make([]*spawn.Territory, 0, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		territory, ok := territories[name]
		if !ok {
			return nil, fmt.Errorf("unknown territory %q", name)
		}
		resolved = append(resolved, territory)
	}
	return resolved, nil
}
