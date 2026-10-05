package xml

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/manor"
)

type manorFile struct {
	Manors []manorElement `xml:"manor"`
}

type manorElement struct {
	ID    *literal32     `xml:"id,attr"`
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
			seed, err := buildSeed(newAttrValues(foldAttrs(crop.Attrs), ""), id)
			if err != nil {
				return nil, fmt.Errorf("xml: %s: manor %d: %w", path, id, err)
			}
			seeds = append(seeds, seed)
		}
		manors = append(manors, manor.Manor{ID: id, Name: el.Name, Seeds: seeds})
	}
	return manor.NewTable(manors), nil
}

// buildSeed builds one <crop> of the manor whose castle id is castleID.
// Every attribute but isAlternative is a required decimal int.
func buildSeed(a *attrValues, castleID int) (manor.Seed, error) {
	a.prefix = "manor: seed"
	cropID := a.int("id")
	if err := a.Err(); err != nil {
		return manor.Seed{}, err
	}
	a.prefix = fmt.Sprintf("manor: seed crop %d", cropID)
	seed := manor.Seed{
		CropID:      cropID,
		SeedID:      a.int("seedId"),
		MatureID:    a.int("matureId"),
		Level:       a.int("level"),
		Reward1:     a.int("reward1"),
		Reward2:     a.int("reward2"),
		CastleID:    castleID,
		Alternative: a.boolDefault("isAlternative", false),
		SeedsLimit:  a.int("seedsLimit"),
		CropsLimit:  a.int("cropsLimit"),
	}
	if err := a.Err(); err != nil {
		return manor.Seed{}, err
	}
	return seed, nil
}

type manorAreaFile struct {
	Areas []manorAreaElement `xml:"area"`
}

type manorAreaElement struct {
	Name     string         `xml:"name,attr"`
	CastleID *literal32     `xml:"castleId,attr"`
	MinZ     *literal32     `xml:"minZ,attr"`
	MaxZ     *literal32     `xml:"maxZ,attr"`
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
		if el.CastleID == nil {
			return nil, fmt.Errorf("xml: %s: manor area %q: castleId is required", path, el.Name)
		}
		if el.MinZ == nil {
			return nil, fmt.Errorf("xml: %s: manor area %q: minZ is required", path, el.Name)
		}
		if el.MaxZ == nil {
			return nil, fmt.Errorf("xml: %s: manor area %q: maxZ is required", path, el.Name)
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
