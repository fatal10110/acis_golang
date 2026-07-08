package xml

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/restart"
)

type restartFile struct {
	Areas  []restartAreaElement  `xml:"area"`
	Points []restartPointElement `xml:"point"`
}

type restartAreaElement struct {
	MinZ     int                         `xml:"minZ,attr"`
	MaxZ     int                         `xml:"maxZ,attr"`
	Nodes    []attrsElement              `xml:"node"`
	Restarts []restartRestrictionElement `xml:"restart"`
}

type restartRestrictionElement struct {
	Race string `xml:"race,attr"`
	Zone string `xml:"zone,attr"`
}

type restartPointElement struct {
	Sets []restartSetElement `xml:"set"`
}

type restartSetElement struct {
	Name string `xml:"name,attr"`
	Val  string `xml:"val,attr"`
}

// LoadRestartPoints parses restart areas and region-scale restart points.
func LoadRestartPoints(path string) (*restart.Table, error) {
	var doc restartFile
	if err := readXML(path, &doc); err != nil {
		return nil, err
	}

	areas := make([]restart.Area, 0, len(doc.Areas))
	for _, el := range doc.Areas {
		area, err := buildRestartArea(el)
		if err != nil {
			return nil, fmt.Errorf("xml: %s: %w", path, err)
		}
		areas = append(areas, area)
	}

	points := make([]restart.Point, 0, len(doc.Points))
	for _, el := range doc.Points {
		point, err := buildRestartPoint(el)
		if err != nil {
			return nil, fmt.Errorf("xml: %s: %w", path, err)
		}
		points = append(points, point)
	}
	return &restart.Table{Areas: areas, Points: points}, nil
}

func buildRestartArea(el restartAreaElement) (restart.Area, error) {
	nodes := make([]location.Point, 0, len(el.Nodes))
	for _, node := range el.Nodes {
		point, err := location.NewPoint(commons.StatSetFromXMLAttrs(node.Attrs))
		if err != nil {
			return restart.Area{}, err
		}
		nodes = append(nodes, point)
	}
	restrictions := make(map[player.Race]string, len(el.Restarts))
	for _, r := range el.Restarts {
		race, err := restart.ParseRace(r.Race)
		if err != nil {
			return restart.Area{}, err
		}
		restrictions[race] = r.Zone
	}
	return restart.Area{MinZ: el.MinZ, MaxZ: el.MaxZ, Nodes: nodes, Restrictions: restrictions}, nil
}

func buildRestartPoint(el restartPointElement) (restart.Point, error) {
	set := commons.NewStatSet()
	var points []location.Location
	var chaoPoints []location.Location
	var mapRegions []location.Point
	for _, s := range el.Sets {
		switch s.Name {
		case "point":
			loc, err := restart.ParseLocationValue(s.Val)
			if err != nil {
				return restart.Point{}, err
			}
			points = append(points, loc)
		case "chaoPoint":
			loc, err := restart.ParseLocationValue(s.Val)
			if err != nil {
				return restart.Point{}, err
			}
			chaoPoints = append(chaoPoints, loc)
		case "map":
			point, err := restart.ParsePointValue(s.Val)
			if err != nil {
				return restart.Point{}, err
			}
			mapRegions = append(mapRegions, point)
		default:
			set.Set(s.Name, s.Val)
		}
	}
	set.Set("points", points)
	set.Set("chaoPoints", chaoPoints)
	set.Set("mapRegions", mapRegions)
	return restart.NewPoint(set)
}
