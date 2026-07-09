package xml

import (
	"encoding/xml"
	"fmt"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/residence"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/residence/castle"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/residence/clanhall"
)

type castleFile struct {
	Castles []castleElement `xml:"castle"`
}

type castleElement struct {
	Attrs         []xml.Attr             `xml:",any,attr"`
	Artifacts     []attrsElement         `xml:"artifacts>artifact"`
	ControlTowers []castleTowerElement   `xml:"controlTowers>controlTower"`
	Gates         []attrsElement         `xml:"gates"`
	NPCs          []attrsElement         `xml:"npcs"`
	Spawns        []attrsElement         `xml:"spawns>spawn"`
	Taxes         []attrsElement         `xml:"tax"`
	Tickets       []attrsElement         `xml:"tickets>ticket"`
	Zones         []residenceZoneElement `xml:"zones>zone"`
}

type castleTowerElement struct {
	Attrs    []xml.Attr     `xml:",any,attr"`
	Position []attrsElement `xml:"position"`
	Stats    []attrsElement `xml:"stats"`
	Zones    []attrsElement `xml:"zones"`
}

type clanHallFile struct {
	Halls []clanHallElement `xml:"clanHall"`
}

type clanHallElement struct {
	Attrs  []xml.Attr             `xml:",any,attr"`
	Agits  []attrsElement         `xml:"agit"`
	Gates  []attrsElement         `xml:"gates"`
	NPCs   []attrsElement         `xml:"npcs"`
	Spawns []attrsElement         `xml:"spawns>spawn"`
	Taxes  []attrsElement         `xml:"tax"`
	Zones  []residenceZoneElement `xml:"zones>zone"`
}

type clanHallDecoFile struct {
	Decos []attrsElement `xml:"deco"`
}

type residenceZoneElement struct {
	Attrs []xml.Attr     `xml:",any,attr"`
	Nodes []attrsElement `xml:"node"`
}

// LoadCastles parses castles.xml into static castle data.
func LoadCastles(path string) (*castle.Table, error) {
	var doc castleFile
	if err := readXML(path, &doc); err != nil {
		return nil, fmt.Errorf("castles: %w", err)
	}

	castles := make([]*castle.Castle, 0, len(doc.Castles))
	for _, el := range doc.Castles {
		entry, err := buildCastle(el)
		if err != nil {
			return nil, fmt.Errorf("xml: %s: %w", path, err)
		}
		castles = append(castles, entry)
	}
	table, err := castle.NewTable(castles)
	if err != nil {
		return nil, fmt.Errorf("xml: %s: %w", path, err)
	}
	return table, nil
}

// LoadClanHalls parses clanHalls.xml into static clan hall data.
func LoadClanHalls(path string) (*clanhall.Table, error) {
	var doc clanHallFile
	if err := readXML(path, &doc); err != nil {
		return nil, fmt.Errorf("clan halls: %w", err)
	}

	halls := make([]*clanhall.Hall, 0, len(doc.Halls))
	for _, el := range doc.Halls {
		entry, err := buildClanHall(el)
		if err != nil {
			return nil, fmt.Errorf("xml: %s: %w", path, err)
		}
		halls = append(halls, entry)
	}
	table, err := clanhall.NewTable(halls)
	if err != nil {
		return nil, fmt.Errorf("xml: %s: %w", path, err)
	}
	return table, nil
}

// LoadClanHallDeco parses clanHallDeco.xml into lookupable decoration data.
func LoadClanHallDeco(path string) (*clanhall.DecoTable, error) {
	var doc clanHallDecoFile
	if err := readXML(path, &doc); err != nil {
		return nil, fmt.Errorf("clan hall deco: %w", err)
	}

	decos := make([]clanhall.Deco, 0, len(doc.Decos))
	for _, el := range doc.Decos {
		entry, err := clanhall.NewDeco(commons.StatSetFromXMLAttrs(el.Attrs))
		if err != nil {
			return nil, fmt.Errorf("xml: %s: %w", path, err)
		}
		decos = append(decos, entry)
	}
	table, err := clanhall.NewDecoTable(decos)
	if err != nil {
		return nil, fmt.Errorf("xml: %s: %w", path, err)
	}
	return table, nil
}

func buildCastle(el castleElement) (*castle.Castle, error) {
	set := commons.StatSetFromXMLAttrs(el.Attrs)
	mergeSingleValue(set, "gates", el.Gates)
	mergeSingleValue(set, "npcs", el.NPCs)
	for _, taxEl := range el.Taxes {
		set.MergeXMLAttrs(taxEl.Attrs)
	}

	artifacts := make([]castle.Artifact, 0, len(el.Artifacts))
	for _, artifactEl := range el.Artifacts {
		entry, err := castle.NewArtifact(commons.StatSetFromXMLAttrs(artifactEl.Attrs))
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, entry)
	}

	towers := make([]castle.ControlTower, 0, len(el.ControlTowers))
	for _, towerEl := range el.ControlTowers {
		towerSet := commons.StatSetFromXMLAttrs(towerEl.Attrs)
		for _, posEl := range towerEl.Position {
			towerSet.MergeXMLAttrs(posEl.Attrs)
		}
		for _, statEl := range towerEl.Stats {
			towerSet.MergeXMLAttrs(statEl.Attrs)
		}
		mergeSingleValue(towerSet, "zones", towerEl.Zones)
		entry, err := castle.NewControlTower(towerSet)
		if err != nil {
			return nil, err
		}
		towers = append(towers, entry)
	}

	tickets := make([]castle.Ticket, 0, len(el.Tickets))
	for _, ticketEl := range el.Tickets {
		entry, err := castle.NewTicket(commons.StatSetFromXMLAttrs(ticketEl.Attrs))
		if err != nil {
			return nil, err
		}
		tickets = append(tickets, entry)
	}

	zones, err := buildResidenceZones(el.Zones)
	if err != nil {
		return nil, err
	}
	spawns, err := buildResidenceSpawns(el.Spawns)
	if err != nil {
		return nil, err
	}
	return castle.NewCastle(set, artifacts, towers, tickets, zones, spawns)
}

func buildClanHall(el clanHallElement) (*clanhall.Hall, error) {
	set := commons.StatSetFromXMLAttrs(el.Attrs)
	for _, agitEl := range el.Agits {
		set.MergeXMLAttrs(agitEl.Attrs)
	}
	mergeSingleValue(set, "gates", el.Gates)
	mergeSingleValue(set, "npcs", el.NPCs)
	for _, taxEl := range el.Taxes {
		set.MergeXMLAttrs(taxEl.Attrs)
	}

	zones, err := buildResidenceZones(el.Zones)
	if err != nil {
		return nil, err
	}
	spawns, err := buildResidenceSpawns(el.Spawns)
	if err != nil {
		return nil, err
	}
	return clanhall.NewHall(set, zones, spawns)
}

func buildResidenceZones(elems []residenceZoneElement) ([]residence.Zone, error) {
	zones := make([]residence.Zone, 0, len(elems))
	for _, el := range elems {
		set := commons.StatSetFromXMLAttrs(el.Attrs)
		kind, err := commons.GetEnum(set, "type", residence.ZoneTypeNames)
		if err != nil {
			return nil, err
		}
		minZ, err := set.GetInt("minZ")
		if err != nil {
			return nil, err
		}
		maxZ, err := set.GetInt("maxZ")
		if err != nil {
			return nil, err
		}
		nodes := make([]location.Point, 0, len(el.Nodes))
		for _, nodeEl := range el.Nodes {
			node, err := location.NewPoint(commons.StatSetFromXMLAttrs(nodeEl.Attrs))
			if err != nil {
				return nil, err
			}
			nodes = append(nodes, node)
		}
		zones = append(zones, residence.Zone{
			Type:  kind,
			MinZ:  minZ,
			MaxZ:  maxZ,
			Nodes: nodes,
		})
	}
	return zones, nil
}

func buildResidenceSpawns(elems []attrsElement) (map[residence.SpawnType][]location.Location, error) {
	if len(elems) == 0 {
		return nil, nil
	}
	out := make(map[residence.SpawnType][]location.Location)
	for _, el := range elems {
		set := commons.StatSetFromXMLAttrs(el.Attrs)
		kind, err := commons.GetEnum(set, "type", residence.SpawnTypeNames)
		if err != nil {
			return nil, err
		}
		loc, err := location.NewLocation(set)
		if err != nil {
			return nil, err
		}
		out[kind] = append(out[kind], loc)
	}
	return out, nil
}

func mergeSingleValue(dst *commons.StatSet, key string, elems []attrsElement) {
	if len(elems) == 0 {
		return
	}
	valueSet := commons.StatSetFromXMLAttrs(elems[0].Attrs)
	if value := valueSet.GetStringDefault("val", ""); value != "" {
		dst.Set(key, value)
	}
}
