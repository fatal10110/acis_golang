package xml

import (
	"encoding/xml"
	"fmt"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/engine"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/door"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/rs/zerolog"
)

type doorFile struct {
	Doors []doorElement `xml:"door"`
}

type doorElement struct {
	Attrs       []xml.Attr     `xml:",any,attr"`
	Position    attrsElement   `xml:"position"`
	Coordinates []pointElement `xml:"coordinates>loc"`
	Stats       attrsElement   `xml:"stats"`
	Function    attrsElement   `xml:"function"`
}

// LoadDoors parses doors.xml and returns static door templates. Runtime geo
// object generation and spawning belong to the world/geo slice, not this
// data loader. log receives skipped-door diagnostics.
func LoadDoors(path string, log zerolog.Logger) (*door.Table, error) {
	var doc doorFile
	if err := readXML(path, &doc); err != nil {
		return nil, fmt.Errorf("doors: %w", err)
	}

	templates := make([]*door.Template, 0, len(doc.Doors))
doors:
	for _, el := range doc.Doors {
		tmpl, err := buildDoorTemplate(el)
		if err != nil {
			return nil, fmt.Errorf("xml: %s: %w", path, err)
		}
		for _, point := range tmpl.Coordinates {
			if engine.OutOfWorld(point.X, point.Y) {
				log.Error().Int("door_id", tmpl.ID).Str("file", path).Msg("data/xml: skipping out-of-world door")
				continue doors
			}
		}
		templates = append(templates, tmpl)
	}
	table, err := door.NewTable(templates)
	if err != nil {
		return nil, fmt.Errorf("xml: %s: %w", path, err)
	}
	return table, nil
}

func buildDoorTemplate(el doorElement) (*door.Template, error) {
	set := commons.StatSetFromXMLAttrs(el.Attrs)
	set.MergeXMLAttrs(el.Position.Attrs)
	set.MergeXMLAttrs(el.Stats.Attrs)
	set.MergeXMLAttrs(el.Function.Attrs)

	coords := make([]location.Point, 0, len(el.Coordinates))
	for _, coord := range el.Coordinates {
		point, err := coord.point()
		if err != nil {
			return nil, err
		}
		coords = append(coords, point)
	}
	set.Set("coords", coords)
	return door.NewTemplate(set)
}
