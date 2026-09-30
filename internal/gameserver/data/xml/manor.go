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
	ID    *coord         `xml:"id,attr"`
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
		if el.ID == nil {
			return nil, fmt.Errorf("xml: %s: manor %q: id is required", path, el.Name)
		}
		id := int(*el.ID)
		seeds := make([]manor.Seed, 0, len(el.Crops))
		for _, crop := range el.Crops {
			set := commons.StatSetFromXMLAttrs(crop.Attrs)
			set.Set("castleId", id)
			seed, err := manor.NewSeed(set)
			if err != nil {
				return nil, fmt.Errorf("xml: %s: manor %d: %w", path, id, err)
			}
			seeds = append(seeds, seed)
		}
		manors = append(manors, manor.Manor{ID: id, Name: el.Name, Seeds: seeds})
	}
	return manor.NewTable(manors), nil
}

type manorAreaFile struct {
	Areas []manorAreaElement `xml:"area"`
}

type manorAreaElement struct {
	Name     string         `xml:"name,attr"`
	CastleID *coord         `xml:"castleId,attr"`
	MinZ     *coord         `xml:"minZ,attr"`
	MaxZ     *coord         `xml:"maxZ,attr"`
	Nodes    []pointElement `xml:"node"`
}

// LoadManorAreas parses manor area polygons.
func LoadManorAreas(path string) (manor.AreaTable, error) {
	var doc manorAreaFile
	if err := readXML(path, &doc); err != nil {
		return nil, fmt.Errorf("manor areas: %w", err)
	}

	areas := make(manor.AreaTable, 0, len(doc.Areas))
	for _, el := range doc.Areas {
		if el.CastleID == nil || el.MinZ == nil || el.MaxZ == nil {
			return nil, fmt.Errorf("xml: %s: manor area %q: castleId, minZ and maxZ are required", path, el.Name)
		}
		nodes := make([]location.Point, 0, len(el.Nodes))
		for _, node := range el.Nodes {
			point, err := node.point()
			if err != nil {
				return nil, fmt.Errorf("xml: %s: manor area %q: %w", path, el.Name, err)
			}
			nodes = append(nodes, point)
		}
		areas = append(areas, manor.Area{
			Name:     el.Name,
			CastleID: int(*el.CastleID),
			MinZ:     int(*el.MinZ),
			MaxZ:     int(*el.MaxZ),
			Nodes:    nodes,
		})
	}
	return areas, nil
}
