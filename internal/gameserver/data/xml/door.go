package xml

import (
	"encoding/xml"
	"fmt"

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

// buildDoorTemplate folds a <door> element's own, <position>, <stats> and
// <function> attributes, in that order with later ones winning, and reads
// the template from them. The position coordinates are integer literals;
// the function attributes default to a closed, NPC-opened door with no
// residence link or timers.
func buildDoorTemplate(el doorElement) (*door.Template, error) {
	vals := foldAttrs(el.Attrs)
	for k, v := range foldAttrs(el.Position.Attrs) {
		vals[k] = v
	}
	if err := canonicalizeLiteralAttrs(vals, "x", "y", "z"); err != nil {
		return nil, fmt.Errorf("door position: %w", err)
	}
	for _, attrs := range [][]xml.Attr{el.Stats.Attrs, el.Function.Attrs} {
		for k, v := range foldAttrs(attrs) {
			vals[k] = v
		}
	}

	a := newAttrValues(vals, "door: template")
	id := a.int("id")
	if err := a.Err(); err != nil {
		return nil, err
	}
	a.prefix = fmt.Sprintf("door: template %d", id)

	t := &door.Template{
		ID:       id,
		Name:     a.strDefault("name", ""),
		Kind:     attrEnum(a, "type", door.ParseKind),
		Level:    a.int("level"),
		Position: location.Location{X: a.int("x"), Y: a.int("y"), Z: a.int("z")},

		HP:     a.int("hp"),
		PDef:   a.int("pDef"),
		MDef:   a.int("mDef"),
		Height: a.int("height"),

		CastleID:    a.intDefault("castle", 0),
		ClanHallID:  a.intDefault("clanHall", 0),
		TriggeredID: a.intDefault("triggeredId", 0),
		Opened:      a.boolDefault("opened", false),
		OpenKind:    attrEnumDefault(a, "openType", door.ParseOpenKind, door.OpenNPC),
		OpenTime:    a.intDefault("openTime", 0),
		RandomTime:  a.intDefault("randomTime", 0),
		CloseTime:   a.intDefault("closeTime", 0),
	}
	if t.Name == "" {
		a.fail(fmt.Errorf("name is required"))
	}
	for _, c := range el.Coordinates {
		point, err := c.point()
		if err != nil {
			return nil, err
		}
		t.Coordinates = append(t.Coordinates, point)
	}
	if len(t.Coordinates) < 3 {
		a.fail(fmt.Errorf("coords requires at least 3 points"))
	}
	if err := a.Err(); err != nil {
		return nil, err
	}
	return t, nil
}
