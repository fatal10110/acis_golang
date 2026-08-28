package xml

import (
	"encoding/xml"
	"errors"
	"fmt"
	"strings"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/rs/zerolog"
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
//
// log receives skipped-territory diagnostics; the zero logger discards them.
func LoadSpawnlist(dir string, log zerolog.Logger) (*spawn.Table, error) {
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
				if errors.Is(err, spawn.ErrTerritoryBuild) {
					log.Warn().Err(err).Str("file", doc.Path).Msg("data/xml: skipping unbuildable territory")
					continue
				}
				return nil, fmt.Errorf("data/xml: parse territory in %s: %w", doc.Path, err)
			}
			// SpawnManager stores every declared territory in a Set with no
			// equals/hashCode override, so conflicting same-name
			// declarations are retained rather than rejected; lookups
			// resolve to an unspecified (here: first-declared) match. Keep
			// every declaration for the table and let the first-declared
			// name win the resolution map, matching that first-wins order.
			key := strings.ToLower(territory.Name)
			if _, exists := territoryMap[key]; !exists {
				territoryMap[key] = territory
			}
			territories = append(territories, territory)
		}
	}

	for _, doc := range docs {
		for _, el := range doc.Data.Makers {
			maker, err := buildMaker(el, territoryMap, log)
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

func buildMaker(el makerElement, territories map[string]*spawn.Territory, log zerolog.Logger) (*spawn.Maker, error) {
	set := commons.StatSetFromXMLAttrs(el.Attrs)
	f := commons.NewFields(set, "spawn maker loader")
	name := f.StringDefault("name", "?")

	refs := resolveTerritories(name, f.StringDefault("territory", ""), territories, log)
	banned := resolveTerritories(name, f.StringDefault("ban", ""), territories, log)

	aiType, aiParams := flattenAI(el.AI, true)
	if aiType != "" {
		set.Set("maker", aiType)
	}

	entries := make([]spawn.Entry, 0, len(el.NPCs))
	for _, npcEl := range el.NPCs {
		entry, err := buildEntry(npcEl)
		if err != nil {
			return nil, fmt.Errorf("maker %q: %w", name, err)
		}
		entries = append(entries, entry)
	}

	return spawn.NewMaker(set, refs, banned, entries, aiParams)
}

func buildEntry(el spawnNPCElement) (spawn.Entry, error) {
	set := commons.StatSetFromXMLAttrs(el.Attrs)
	f := commons.NewFields(set, "spawn entry loader")
	npcID := f.StringDefault("id", "?")

	privates := make([]spawn.Private, 0)
	for _, group := range el.Privates {
		for _, privateEl := range group.Entries {
			privateSpawn, err := spawn.NewPrivate(commons.StatSetFromXMLAttrs(privateEl.Attrs))
			if err != nil {
				return spawn.Entry{}, fmt.Errorf("npc %q private: %w", npcID, err)
			}
			privates = append(privates, privateSpawn)
		}
	}

	_, aiParams := flattenAI(el.AI, false)
	positions, err := spawn.ParsePositions(f.StringDefault("pos", ""))
	if err != nil {
		return spawn.Entry{}, fmt.Errorf("npc %q: %w", npcID, err)
	}

	entry, err := spawn.NewEntry(set, positions, privates, aiParams)
	if err != nil {
		return spawn.Entry{}, fmt.Errorf("npc %q: %w", npcID, err)
	}
	return entry, nil
}

func flattenAI(ai []aiElement, stripAt bool) (string, map[string]string) {
	var kind string
	params := make(map[string]string)
	for _, el := range ai {
		set := commons.StatSetFromXMLAttrs(el.Attrs)
		kind = set.GetStringDefault("type", kind)
		for _, param := range el.Sets {
			value := param.Val
			if stripAt {
				value = strings.ReplaceAll(value, "@", "")
			}
			params[param.Name] = value
		}
	}
	if len(params) == 0 {
		params = nil
	}
	return kind, params
}

// resolveTerritories looks up each ";"-delimited name in raw, matching
// SpawnManager.findTerritory: a single name resolves to null (here: dropped,
// with a warning) if unknown, exactly like getTerritory. A multi-name group
// is all-or-nothing — findTerritory logs once and returns null for the whole
// group the moment any member is missing, rather than building a partial
// composite (SpawnManager.java:500-537) — so an unresolved name here drops
// every name in that group, not just the missing one.
func resolveTerritories(makerName, raw string, territories map[string]*spawn.Territory, log zerolog.Logger) []*spawn.Territory {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var names []string
	for _, name := range strings.Split(raw, ";") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		names = append(names, name)
	}

	resolved := make([]*spawn.Territory, 0, len(names))
	for _, name := range names {
		territory, ok := territories[strings.ToLower(name)]
		if !ok {
			log.Warn().Str("maker", makerName).Str("territory", name).Msg("data/xml: territory does not exist")
			if len(names) > 1 {
				return nil
			}
			continue
		}
		resolved = append(resolved, territory)
	}
	return resolved
}
