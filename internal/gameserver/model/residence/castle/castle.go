// Package castle contains static castle data loaded from castles.xml.
package castle

import (
	"fmt"
	"strings"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/residence"
)

// TowerType classifies one castle control tower.
type TowerType uint8

const (
	TowerLifeControl TowerType = iota
	TowerTrapControl
)

var towerTypeNames = map[string]TowerType{
	"LIFE_CONTROL": TowerLifeControl,
	"TRAP_CONTROL": TowerTrapControl,
}

var towerTypeStrings = [...]string{"LIFE_CONTROL", "TRAP_CONTROL"}

// String returns the canonical XML spelling for t.
func (t TowerType) String() string {
	if int(t) < len(towerTypeStrings) {
		return towerTypeStrings[t]
	}
	return fmt.Sprintf("TowerType(%d)", uint8(t))
}

// Artifact is one holy artifact spawn entry.
type Artifact struct {
	NPCID    int
	Position location.Location
	Heading  int
}

// ControlTower is one control tower entry.
type ControlTower struct {
	Alias    string
	Type     TowerType
	Position location.Location
	HP       float64
	PDef     float64
	MDef     float64
	Zones    []string
}

// Ticket is one mercenary ticket entry.
type Ticket struct {
	ItemID     int
	Kind       string
	Stationary bool
	NPCID      int
	MaxAmount  int
	SSQ        []string
}

// Castle is one static castle definition from castles.xml.
type Castle struct {
	ID        int
	ParentID  int
	Alias     string
	Name      string
	CircletID int
	Tax       residence.Tax

	Gates  []string
	NPCs   []int
	Spawns map[residence.SpawnType][]location.Location
	Zones  []residence.Zone

	Artifacts     []Artifact
	ControlTowers []ControlTower
	Tickets       []Ticket
}

// NewArtifact builds an Artifact from set.
func NewArtifact(set *commons.StatSet) (Artifact, error) {
	npcID, err := set.GetInt("id")
	if err != nil {
		return Artifact{}, fmt.Errorf("castle: artifact: %w", err)
	}
	posRaw := set.GetStringDefault("pos", "")
	if posRaw == "" {
		return Artifact{}, fmt.Errorf("castle: artifact %d: pos is required", npcID)
	}
	pos, heading, err := parseSpawnLocation(posRaw)
	if err != nil {
		return Artifact{}, fmt.Errorf("castle: artifact %d: %w", npcID, err)
	}
	return Artifact{NPCID: npcID, Position: pos, Heading: heading}, nil
}

// NewControlTower builds a ControlTower from set.
func NewControlTower(set *commons.StatSet) (ControlTower, error) {
	alias := commons.NewFields(set, "castle: control tower").StringDefault("alias", "")
	if alias == "" {
		return ControlTower{}, fmt.Errorf("castle: control tower: alias is required")
	}
	f := commons.NewFields(set, fmt.Sprintf("castle: control tower %q", alias))
	towerType := commons.FieldEnum[TowerType](f, "type", towerTypeNames)
	if err := f.Err(); err != nil {
		return ControlTower{}, err
	}
	pos, err := location.NewLocation(commons.NewStatSetFrom(set))
	if err != nil {
		return ControlTower{}, fmt.Errorf("castle: control tower %q: %w", alias, err)
	}
	tower := ControlTower{
		Alias:    alias,
		Type:     towerType,
		Position: pos,
		HP:       f.Float64("hp"),
		PDef:     f.Float64("pDef"),
		MDef:     f.Float64("mDef"),
		Zones:    cleanStrings(f.StringArrayDefault("zones", nil)),
	}
	if err := f.Err(); err != nil {
		return ControlTower{}, err
	}
	return tower, nil
}

// NewTicket builds a Ticket from set.
func NewTicket(set *commons.StatSet) (Ticket, error) {
	idf := commons.NewFields(set, "castle: ticket")
	itemID := idf.Int("itemId")
	if err := idf.Err(); err != nil {
		return Ticket{}, err
	}

	f := commons.NewFields(set, fmt.Sprintf("castle: ticket %d", itemID))
	kind := f.StringDefault("type", "")
	if kind == "" {
		f.Fail(fmt.Errorf("type is required"))
	}
	ticket := Ticket{
		ItemID:     itemID,
		Kind:       kind,
		Stationary: f.BoolDefault("stationary", false),
		NPCID:      f.Int("npcId"),
		MaxAmount:  f.Int("maxAmount"),
		SSQ:        cleanStrings(f.StringArrayDefault("ssq", nil)),
	}
	if err := f.Err(); err != nil {
		return Ticket{}, err
	}
	return ticket, nil
}

// NewCastle builds a Castle from its XML attrs plus already-decoded child data.
func NewCastle(set *commons.StatSet, artifacts []Artifact, towers []ControlTower, tickets []Ticket, zones []residence.Zone, spawns map[residence.SpawnType][]location.Location) (*Castle, error) {
	idf := commons.NewFields(set, "castle")
	id := idf.Int("id")
	if err := idf.Err(); err != nil {
		return nil, err
	}

	f := commons.NewFields(set, fmt.Sprintf("castle %d", id))
	parentID := f.Int("parentId")
	circletID := f.Int("circletId")
	taxRate := f.Int("taxRate")
	taxSysgetRate := f.Int("taxSysgetRate")
	tributeRate := f.Int("tributeRate")
	alias := f.StringDefault("alias", "")
	if alias == "" {
		f.Fail(fmt.Errorf("alias is required"))
	}
	name := f.StringDefault("name", "")
	if name == "" {
		f.Fail(fmt.Errorf("name is required"))
	}
	npcs := f.IntArray("npcs")
	gates := cleanStrings(f.StringArrayDefault("gates", nil))
	if err := f.Err(); err != nil {
		return nil, err
	}

	return &Castle{
		ID:        id,
		ParentID:  parentID,
		Alias:     alias,
		Name:      name,
		CircletID: circletID,
		Tax: residence.Tax{
			Rate:        taxRate,
			SysgetRate:  taxSysgetRate,
			TributeRate: tributeRate,
		},
		Gates:         gates,
		NPCs:          append([]int(nil), npcs...),
		Spawns:        copySpawns(spawns),
		Zones:         append([]residence.Zone(nil), zones...),
		Artifacts:     append([]Artifact(nil), artifacts...),
		ControlTowers: append([]ControlTower(nil), towers...),
		Tickets:       append([]Ticket(nil), tickets...),
	}, nil
}

// Table stores castles keyed by id and alias.
type Table struct {
	byID    map[int]*Castle
	byAlias map[string]*Castle
	order   []*Castle
}

// NewTable builds a castle table and rejects duplicate ids or aliases.
func NewTable(castles []*Castle) (*Table, error) {
	t := &Table{
		byID:    make(map[int]*Castle, len(castles)),
		byAlias: make(map[string]*Castle, len(castles)),
		order:   make([]*Castle, 0, len(castles)),
	}
	for _, entry := range castles {
		if entry == nil {
			return nil, fmt.Errorf("castle: nil entry")
		}
		if _, exists := t.byID[entry.ID]; exists {
			return nil, fmt.Errorf("castle: duplicate id %d", entry.ID)
		}
		aliasKey := strings.ToLower(entry.Alias)
		if _, exists := t.byAlias[aliasKey]; exists {
			return nil, fmt.Errorf("castle: duplicate alias %q", entry.Alias)
		}
		t.byID[entry.ID] = entry
		t.byAlias[aliasKey] = entry
		t.order = append(t.order, entry)
	}
	return t, nil
}

// Len returns the number of loaded castles.
func (t *Table) Len() int {
	if t == nil {
		return 0
	}
	return len(t.order)
}

// Get returns the castle with id.
func (t *Table) Get(id int) (*Castle, bool) {
	if t == nil {
		return nil, false
	}
	entry, ok := t.byID[id]
	return entry, ok
}

// ByAlias returns the castle with alias, case-insensitively.
func (t *Table) ByAlias(alias string) (*Castle, bool) {
	if t == nil {
		return nil, false
	}
	entry, ok := t.byAlias[strings.ToLower(alias)]
	return entry, ok
}

// All returns the loaded castles in file order.
func (t *Table) All() []*Castle {
	if t == nil {
		return nil
	}
	return append([]*Castle(nil), t.order...)
}

func copySpawns(src map[residence.SpawnType][]location.Location) map[residence.SpawnType][]location.Location {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[residence.SpawnType][]location.Location, len(src))
	for kind, list := range src {
		dst[kind] = append([]location.Location(nil), list...)
	}
	return dst
}

func cleanStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		out = append(out, s)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func parseSpawnLocation(raw string) (location.Location, int, error) {
	parts := strings.Split(raw, ";")
	if len(parts) != 4 {
		return location.Location{}, 0, fmt.Errorf("pos requires x;y;z;heading")
	}
	set := commons.NewStatSetWithCapacity(4)
	set.Set("x", parts[0])
	set.Set("y", parts[1])
	set.Set("z", parts[2])
	pos, err := location.NewLocation(set)
	if err != nil {
		return location.Location{}, 0, err
	}
	set = commons.NewStatSetWithCapacity(1)
	set.Set("heading", parts[3])
	heading, err := set.GetInt("heading")
	if err != nil {
		return location.Location{}, 0, err
	}
	return pos, heading, nil
}
