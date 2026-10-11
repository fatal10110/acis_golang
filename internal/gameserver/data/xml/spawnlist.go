package xml

import (
	"encoding/xml"
	"errors"
	"fmt"
	"strings"

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
// spawnMultiplier is the npcs.properties SpawnMultiplier (default 1): it
// scales and rounds maker maximums and coordinate-less entry totals (see
// spawn.NewMaker and spawn.NewEntry).
func LoadSpawnlist(dir string, log zerolog.Logger, spawnMultiplier float64) (*spawn.Table, error) {
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
			maker, err := buildMaker(el, territoryMap, log, spawnMultiplier)
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
		a := newAttrValues(foldAttrs(nodeEl.Attrs), "spawn territory node")
		node := spawn.Node{X: int(a.int32Literal("x")), Y: int(a.int32Literal("y"))}
		if err := a.Err(); err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}
	a := newAttrValues(foldAttrs(el.Attrs), "spawn territory")
	name := a.str("name")
	if err := a.Err(); err != nil {
		return nil, err
	}
	a.prefix = fmt.Sprintf("spawn territory %q", name)
	minZ := int(a.int32Literal("minZ"))
	maxZ := int(a.int32Literal("maxZ"))
	if err := a.Err(); err != nil {
		return nil, err
	}
	return spawn.NewTerritory(name, minZ, maxZ, nodes)
}

func buildMaker(el makerElement, territories map[string]*spawn.Territory, log zerolog.Logger, spawnMultiplier float64) (*spawn.Maker, error) {
	a := newAttrValues(foldAttrs(el.Attrs), "spawn maker")
	logName := a.strDefault("name", "?")

	refs := resolveTerritories(logName, a.strDefault("territory", ""), territories, log)
	banned := resolveTerritories(logName, a.strDefault("ban", ""), territories, log)

	aiType, aiParams := flattenAI(el.AI, true)
	if aiType == "" {
		aiType = a.strDefault("maker", "")
	}

	entries := make([]spawn.Entry, 0, len(el.NPCs))
	for _, npcEl := range el.NPCs {
		entry, err := buildEntry(npcEl, spawnMultiplier)
		if err != nil {
			return nil, fmt.Errorf("maker %q: %w", logName, err)
		}
		entries = append(entries, entry)
	}

	name := a.str("name")
	if err := a.Err(); err != nil {
		return nil, err
	}
	a.prefix = fmt.Sprintf("spawn maker %q", name)
	maker := spawn.Maker{
		Name:              name,
		Territories:       refs,
		BannedTerritories: banned,
		AIType:            aiType,
		AIParams:          aiParams,
		MaximumNPCs:       a.int("maximumNpcs"),
		Event:             a.strDefault("event", ""),
		SpawnTime:         a.strDefault("spawnTime", ""),
		Entries:           entries,
	}
	if err := a.Err(); err != nil {
		return nil, err
	}
	return spawn.NewMaker(maker, spawnMultiplier), nil
}

func buildEntry(el spawnNPCElement, spawnMultiplier float64) (spawn.Entry, error) {
	a := newAttrValues(foldAttrs(el.Attrs), "spawn entry")
	npcID := a.strDefault("id", "?")
	if err := canonicalizeLiteralAttrs(a.vals, "id", "total"); err != nil {
		return spawn.Entry{}, fmt.Errorf("npc %q: %w", npcID, err)
	}

	privates := make([]spawn.Private, 0)
	for _, group := range el.Privates {
		for _, privateEl := range group.Entries {
			privateSpawn, err := buildSpawnPrivate(newAttrValues(foldAttrs(privateEl.Attrs), "spawn private"))
			if err != nil {
				return spawn.Entry{}, fmt.Errorf("npc %q private: %w", npcID, err)
			}
			privates = append(privates, privateSpawn)
		}
	}

	_, aiParams := flattenAI(el.AI, false)
	positions, err := spawn.ParsePositions(a.strDefault("pos", ""))
	if err != nil {
		return spawn.Entry{}, fmt.Errorf("npc %q: %w", npcID, err)
	}

	id := a.int32("id")
	if err := a.Err(); err != nil {
		return spawn.Entry{}, fmt.Errorf("npc %q: %w", npcID, err)
	}
	a.prefix = fmt.Sprintf("spawn entry %d", id)
	entry := spawn.Entry{
		NPCID:         id,
		Total:         a.int("total"),
		RespawnDelay:  a.gameDuration("respawn"),
		RespawnRandom: a.gameDuration("respawnRand"),
		Positions:     positions,
		Privates:      privates,
		AIParams:      aiParams,
		DBName:        a.strDefault("dbName", ""),
	}
	if a.has("dbSaving") {
		entry.DBSaving = a.strArray("dbSaving")
	}
	if err := a.Err(); err != nil {
		return spawn.Entry{}, fmt.Errorf("npc %q: %w", npcID, err)
	}
	return spawn.NewEntry(entry, spawnMultiplier), nil
}

// buildSpawnPrivate builds one private spawn row under a spawn entry's
// <privates> block. id and weight are required; respawn defaults to 0.
func buildSpawnPrivate(a *attrValues) (spawn.Private, error) {
	p := spawn.Private{
		NPCID:        a.int32("id"),
		Weight:       a.int("weight"),
		RespawnDelay: a.gameDuration("respawn"),
	}
	if err := a.Err(); err != nil {
		return spawn.Private{}, err
	}
	return p, nil
}

func flattenAI(ai []aiElement, stripAt bool) (string, map[string]string) {
	var kind string
	params := make(map[string]string)
	for _, el := range ai {
		if t, ok := foldAttrs(el.Attrs)["type"]; ok {
			kind = t
		}
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

// resolveTerritories looks up each ";"-delimited name in raw: an unknown
// single name resolves to nothing (dropped, with a warning). A multi-name
// group is all-or-nothing — one warning and no territory for the whole group
// the moment any member is missing, rather than a partial composite — so an
// unresolved name here drops every name in that group, not just the missing
// one.
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
