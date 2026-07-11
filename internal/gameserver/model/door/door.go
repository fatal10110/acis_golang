package door

import (
	"fmt"
	"sync"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/geo/block"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// Kind classifies a door template as a regular door or a wall.
type Kind uint8

const (
	KindDoor Kind = iota
	KindWall
)

var kindNames = map[string]Kind{
	"DOOR": KindDoor,
	"WALL": KindWall,
}

var kindStrings = [...]string{"DOOR", "WALL"}

// String returns k's canonical XML spelling.
func (k Kind) String() string {
	if int(k) < len(kindStrings) {
		return kindStrings[k]
	}
	return fmt.Sprintf("Kind(%d)", uint8(k))
}

// OpenKind classifies what opens a door.
type OpenKind uint8

const (
	OpenClick OpenKind = iota
	OpenTime
	OpenSkill
	OpenNPC
)

var openKindNames = map[string]OpenKind{
	"CLICK": OpenClick,
	"TIME":  OpenTime,
	"SKILL": OpenSkill,
	"NPC":   OpenNPC,
}

var openKindStrings = [...]string{"CLICK", "TIME", "SKILL", "NPC"}

// String returns o's canonical XML spelling.
func (o OpenKind) String() string {
	if int(o) < len(openKindStrings) {
		return openKindStrings[o]
	}
	return fmt.Sprintf("OpenKind(%d)", uint8(o))
}

// Template is one static door entry from doors.xml.
type Template struct {
	ID    int
	Name  string
	Kind  Kind
	Level int

	Position    location.Location
	Coordinates []location.Point

	HP, PDef, MDef, Height int

	CastleID, ClanHallID, TriggeredID int
	Opened                            bool
	OpenKind                          OpenKind
	OpenTime, RandomTime, CloseTime   int
}

// GeoShape is the geodata footprint calculated for a door.
type GeoShape interface {
	GeoX() int
	GeoY() int
	GeoZ() int
	Height() int
	GeoData() [][]block.NSWE
}

// Object is one live door spawned into the world.
//
// mu guards opened. Position and visibility are guarded by the embedded
// world.Presence.
type Object struct {
	world.Presence

	objectID int32
	Template *Template

	geoX, geoY, geoZ int
	height           int
	geoData          [][]block.NSWE

	mu     sync.RWMutex
	opened bool
}

// NewObject creates a live door object from a static template and geodata shape.
func NewObject(objectID int32, tmpl *Template, shape GeoShape) (*Object, error) {
	if tmpl == nil {
		return nil, fmt.Errorf("door: nil template")
	}
	if shape == nil {
		return nil, fmt.Errorf("door %d: nil geo shape", tmpl.ID)
	}
	data := shape.GeoData()
	if len(data) == 0 || len(data[0]) == 0 {
		return nil, fmt.Errorf("door %d: empty geo shape", tmpl.ID)
	}

	return &Object{
		objectID: objectID,
		Template: tmpl,
		geoX:     shape.GeoX(),
		geoY:     shape.GeoY(),
		geoZ:     shape.GeoZ(),
		height:   shape.Height(),
		geoData:  cloneGeoData(data),
		opened:   tmpl.Opened,
	}, nil
}

// ObjectID returns the world object id assigned to this door.
func (o *Object) ObjectID() int32 { return o.objectID }

// DoorID returns the static door id from doors.xml.
func (o *Object) DoorID() int { return o.Template.ID }

// MaxHP returns the door's maximum HP.
func (o *Object) MaxHP() int { return o.Template.HP }

// HP returns the door's current HP. Door damage is not modeled yet, so live
// doors currently stay at full HP.
func (o *Object) HP() int { return o.Template.HP }

// Damage returns the visual damage stage sent in door status packets.
func (o *Object) Damage() int { return 0 }

// Opened reports whether this door is currently open.
func (o *Object) Opened() bool {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.opened
}

// SetOpened updates the door's open state and reports whether it changed.
func (o *Object) SetOpened(open bool) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.opened == open {
		return false
	}
	o.opened = open
	return true
}

// GeoX returns the door footprint's starting geodata X coordinate.
func (o *Object) GeoX() int { return o.geoX }

// GeoY returns the door footprint's starting geodata Y coordinate.
func (o *Object) GeoY() int { return o.geoY }

// GeoZ returns the door footprint's baseline geodata Z coordinate.
func (o *Object) GeoZ() int { return o.geoZ }

// Height returns the vertical span the door blocks.
func (o *Object) Height() int { return o.height }

// GeoData returns the door footprint's NSWE edits.
func (o *Object) GeoData() [][]block.NSWE { return cloneGeoData(o.geoData) }

func cloneGeoData(data [][]block.NSWE) [][]block.NSWE {
	out := make([][]block.NSWE, len(data))
	for i := range data {
		out[i] = append([]block.NSWE(nil), data[i]...)
	}
	return out
}

// NewTemplate builds a Template from set. The XML attributes, position,
// coordinates, and stats are required; function attributes use the shipped
// defaults when absent.
func NewTemplate(set *commons.StatSet) (*Template, error) {
	idf := commons.NewFields(set, "door: template")
	id := idf.Int("id")
	if err := idf.Err(); err != nil {
		return nil, err
	}
	wrap := func(err error) error { return fmt.Errorf("door: template %d: %w", id, err) }

	f := commons.NewFields(set, fmt.Sprintf("door: template %d", id))
	kind := commons.FieldEnum[Kind](f, "type", kindNames)
	level := f.Int("level")
	if err := f.Err(); err != nil {
		return nil, err
	}
	position, err := location.NewLocation(commons.NewStatSetFrom(set))
	if err != nil {
		return nil, wrap(err)
	}
	coords := commons.FieldList[location.Point](f, "coords")
	if len(coords) < 3 {
		f.Fail(fmt.Errorf("coords requires at least 3 points"))
	}

	t := &Template{
		ID:          id,
		Name:        f.StringDefault("name", ""),
		Kind:        kind,
		Level:       level,
		Position:    position,
		Coordinates: append([]location.Point(nil), coords...),
	}
	if t.Name == "" {
		f.Fail(fmt.Errorf("name is required"))
	}
	t.HP = f.Int("hp")
	t.PDef = f.Int("pDef")
	t.MDef = f.Int("mDef")
	t.Height = f.Int("height")
	t.CastleID = f.IntDefault("castle", 0)
	t.ClanHallID = f.IntDefault("clanHall", 0)
	t.TriggeredID = f.IntDefault("triggeredId", 0)
	t.Opened = f.BoolDefault("opened", false)
	t.OpenKind = commons.FieldEnumDefault[OpenKind](f, "openType", openKindNames, OpenNPC)
	t.OpenTime = f.IntDefault("openTime", 0)
	t.RandomTime = f.IntDefault("randomTime", 0)
	t.CloseTime = f.IntDefault("closeTime", 0)
	if err := f.Err(); err != nil {
		return nil, err
	}
	return t, nil
}

// Table stores door templates keyed by door id.
type Table struct {
	doors map[int]*Template
	order []*Template
}

// NewTable builds a door table and rejects duplicate ids.
func NewTable(templates []*Template) (*Table, error) {
	t := &Table{
		doors: make(map[int]*Template, len(templates)),
		order: make([]*Template, 0, len(templates)),
	}
	for _, tmpl := range templates {
		if tmpl == nil {
			return nil, fmt.Errorf("door: nil template")
		}
		if _, exists := t.doors[tmpl.ID]; exists {
			return nil, fmt.Errorf("door: duplicate template id %d", tmpl.ID)
		}
		t.doors[tmpl.ID] = tmpl
		t.order = append(t.order, tmpl)
	}
	return t, nil
}

// Len returns the number of loaded door templates.
func (t *Table) Len() int {
	if t == nil {
		return 0
	}
	return len(t.order)
}

// Get returns the template for id.
func (t *Table) Get(id int) (*Template, bool) {
	if t == nil {
		return nil, false
	}
	tmpl, ok := t.doors[id]
	return tmpl, ok
}

// All returns the loaded templates in file order.
func (t *Table) All() []*Template {
	if t == nil {
		return nil
	}
	return append([]*Template(nil), t.order...)
}
