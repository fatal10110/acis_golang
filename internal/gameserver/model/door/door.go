package door

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
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

// NewTemplate builds a Template from set. The XML attributes, position,
// coordinates, and stats are required; function attributes use the shipped
// defaults when absent.
func NewTemplate(set *commons.StatSet) (*Template, error) {
	id, err := set.GetInt("id")
	if err != nil {
		return nil, fmt.Errorf("door: template: %w", err)
	}
	wrap := func(err error) error { return fmt.Errorf("door: template %d: %w", id, err) }

	kind, err := commons.GetEnum(set, "type", kindNames)
	if err != nil {
		return nil, wrap(err)
	}
	level, err := set.GetInt("level")
	if err != nil {
		return nil, wrap(err)
	}
	position, err := location.NewLocation(commons.NewStatSetFrom(set))
	if err != nil {
		return nil, wrap(err)
	}
	coords, err := commons.GetList[location.Point](set, "coords")
	if err != nil {
		return nil, wrap(err)
	}
	if len(coords) < 3 {
		return nil, wrap(fmt.Errorf("coords requires at least 3 points"))
	}

	t := &Template{
		ID:          id,
		Name:        set.GetStringDefault("name", ""),
		Kind:        kind,
		Level:       level,
		Position:    position,
		Coordinates: append([]location.Point(nil), coords...),
	}
	if t.Name == "" {
		return nil, wrap(fmt.Errorf("name is required"))
	}
	if t.HP, err = set.GetInt("hp"); err != nil {
		return nil, wrap(err)
	}
	if t.PDef, err = set.GetInt("pDef"); err != nil {
		return nil, wrap(err)
	}
	if t.MDef, err = set.GetInt("mDef"); err != nil {
		return nil, wrap(err)
	}
	if t.Height, err = set.GetInt("height"); err != nil {
		return nil, wrap(err)
	}
	if t.CastleID, err = set.GetIntDefault("castle", 0); err != nil {
		return nil, wrap(err)
	}
	if t.ClanHallID, err = set.GetIntDefault("clanHall", 0); err != nil {
		return nil, wrap(err)
	}
	if t.TriggeredID, err = set.GetIntDefault("triggeredId", 0); err != nil {
		return nil, wrap(err)
	}
	t.Opened = set.GetBoolDefault("opened", false)
	if t.OpenKind, err = commons.GetEnumDefault(set, "openType", openKindNames, OpenNPC); err != nil {
		return nil, wrap(err)
	}
	if t.OpenTime, err = set.GetIntDefault("openTime", 0); err != nil {
		return nil, wrap(err)
	}
	if t.RandomTime, err = set.GetIntDefault("randomTime", 0); err != nil {
		return nil, wrap(err)
	}
	if t.CloseTime, err = set.GetIntDefault("closeTime", 0); err != nil {
		return nil, wrap(err)
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
