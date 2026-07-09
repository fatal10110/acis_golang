package xml

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/manor"
)

type manorFile struct {
	Manors []manorElement `xml:"manor"`
}

type manorElement struct {
	ID    int            `xml:"id,attr"`
	Name  string         `xml:"name,attr"`
	Crops []attrsElement `xml:"crop"`
}

// LoadManors parses manor seed/crop rows.
func LoadManors(path string) (*manor.Table, error) {
	var doc manorFile
	if err := readXML(path, &doc); err != nil {
		return nil, fmt.Errorf("manors: %w", err)
	}

	manors := make([]manor.Manor, 0, len(doc.Manors))
	for _, el := range doc.Manors {
		seeds := make([]manor.Seed, 0, len(el.Crops))
		for _, crop := range el.Crops {
			set := commons.StatSetFromXMLAttrs(crop.Attrs)
			set.Set("castleId", el.ID)
			seed, err := manor.NewSeed(set)
			if err != nil {
				return nil, fmt.Errorf("xml: %s: manor %d: %w", path, el.ID, err)
			}
			seeds = append(seeds, seed)
		}
		manors = append(manors, manor.Manor{ID: el.ID, Name: el.Name, Seeds: seeds})
	}
	table, err := manor.NewTable(manors)
	if err != nil {
		return nil, fmt.Errorf("xml: %s: %w", path, err)
	}
	return table, nil
}

type manorAreaFile struct {
	Areas []manorAreaElement `xml:"area"`
}

type manorAreaElement struct {
	Name     string         `xml:"name,attr"`
	CastleID int            `xml:"castleId,attr"`
	MinZ     int            `xml:"minZ,attr"`
	MaxZ     int            `xml:"maxZ,attr"`
	Nodes    []attrsElement `xml:"node"`
}

// LoadManorAreas parses manor area polygons.
func LoadManorAreas(path string) (manor.AreaTable, error) {
	var doc manorAreaFile
	if err := readXML(path, &doc); err != nil {
		return nil, fmt.Errorf("manor areas: %w", err)
	}

	areas := make(manor.AreaTable, 0, len(doc.Areas))
	for _, el := range doc.Areas {
		nodes := make([]location.Point, 0, len(el.Nodes))
		for _, node := range el.Nodes {
			point, err := location.NewPoint(commons.StatSetFromXMLAttrs(node.Attrs))
			if err != nil {
				return nil, fmt.Errorf("xml: %s: manor area %q: %w", path, el.Name, err)
			}
			nodes = append(nodes, point)
		}
		areas = append(areas, manor.Area{
			Name:     el.Name,
			CastleID: el.CastleID,
			MinZ:     el.MinZ,
			MaxZ:     el.MaxZ,
			Nodes:    nodes,
		})
	}
	return areas, nil
}
