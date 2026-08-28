// Package castle contains static castle data loaded from castles.xml.
package castle

import (
	"fmt"
	"strconv"
	"strings"

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

// NewArtifact builds an Artifact from its already-decoded npc id and its
// "x;y;z;heading" pos attribute.
func NewArtifact(npcID int, pos string) (Artifact, error) {
	if pos == "" {
		return Artifact{}, fmt.Errorf("castle: artifact %d: pos is required", npcID)
	}
	position, heading, err := parseSpawnLocation(pos)
	if err != nil {
		return Artifact{}, fmt.Errorf("castle: artifact %d: %w", npcID, err)
	}
	return Artifact{NPCID: npcID, Position: position, Heading: heading}, nil
}

// NewControlTower builds a ControlTower from its already-decoded attributes.
func NewControlTower(alias, towerType string, x, y, z int, hp, pDef, mDef float64, zones []string) (ControlTower, error) {
	if alias == "" {
		return ControlTower{}, fmt.Errorf("castle: control tower: alias is required")
	}
	kind, ok := towerTypeNames[towerType]
	if !ok {
		return ControlTower{}, fmt.Errorf("castle: control tower %q: type: unrecognized %q", alias, towerType)
	}
	return ControlTower{
		Alias:    alias,
		Type:     kind,
		Position: location.Location{X: x, Y: y, Z: z},
		HP:       hp,
		PDef:     pDef,
		MDef:     mDef,
		Zones:    zones,
	}, nil
}

// NewTicket builds a Ticket from its already-decoded attributes.
func NewTicket(itemID int, kind string, stationary bool, npcID, maxAmount int, ssq []string) (Ticket, error) {
	if _, ok := ticketTypes[kind]; !ok {
		return Ticket{}, fmt.Errorf("castle: ticket %d: type: unrecognized %q", itemID, kind)
	}
	for _, cabal := range ssq {
		if _, ok := cabalTypes[cabal]; !ok {
			return Ticket{}, fmt.Errorf("castle: ticket %d: ssq: unrecognized %q", itemID, cabal)
		}
	}
	return Ticket{
		ItemID:     itemID,
		Kind:       kind,
		Stationary: stationary,
		NPCID:      npcID,
		MaxAmount:  maxAmount,
		SSQ:        ssq,
	}, nil
}

var ticketTypes = map[string]struct{}{
	"SWORD": {}, "POLE": {}, "BOW": {}, "CLERIC": {}, "WIZARD": {}, "TELEPORTER": {},
}

var cabalTypes = map[string]struct{}{
	"NORMAL": {}, "DAWN": {}, "DUSK": {},
}

// CastleAttrs holds a Castle's own decoded XML attributes, separate from its
// already-decoded child collections (artifacts, towers, tickets, zones,
// spawns) passed alongside to NewCastle.
type CastleAttrs struct {
	ID, ParentID, CircletID int
	Alias, Name             string
	Tax                     residence.Tax
	Gates                   []string
	NPCs                    []int
}

// NewCastle builds a Castle from its decoded attrs plus already-decoded
// child data.
func NewCastle(attrs CastleAttrs, artifacts []Artifact, towers []ControlTower, tickets []Ticket, zones []residence.Zone, spawns map[residence.SpawnType][]location.Location) (*Castle, error) {
	if attrs.Alias == "" {
		return nil, fmt.Errorf("castle %d: alias is required", attrs.ID)
	}
	if attrs.Name == "" {
		return nil, fmt.Errorf("castle %d: name is required", attrs.ID)
	}

	return &Castle{
		ID:            attrs.ID,
		ParentID:      attrs.ParentID,
		Alias:         attrs.Alias,
		Name:          attrs.Name,
		CircletID:     attrs.CircletID,
		Tax:           attrs.Tax,
		Gates:         attrs.Gates,
		NPCs:          append([]int(nil), attrs.NPCs...),
		Spawns:        residence.CopySpawns(spawns),
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

// NewTable builds a castle table, retaining the last entry for a duplicate id.
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
		if old, exists := t.byID[entry.ID]; exists {
			for i, listed := range t.order {
				if listed == old {
					t.order[i] = entry
					break
				}
			}
			if aliasKey := strings.ToLower(old.Alias); t.byAlias[aliasKey] == old {
				delete(t.byAlias, aliasKey)
			}
		} else {
			t.order = append(t.order, entry)
		}
		aliasKey := strings.ToLower(entry.Alias)
		t.byID[entry.ID] = entry
		t.byAlias[aliasKey] = entry
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

func parseSpawnLocation(raw string) (location.Location, int, error) {
	parts := strings.Split(raw, ";")
	if len(parts) != 4 {
		return location.Location{}, 0, fmt.Errorf("pos requires x;y;z;heading")
	}
	var nums [4]int
	for i, name := range []string{"x", "y", "z", "heading"} {
		n, err := strconv.Atoi(parts[i])
		if err != nil {
			return location.Location{}, 0, fmt.Errorf("castle: pos %s: %w", name, err)
		}
		nums[i] = n
	}
	return location.Location{X: nums[0], Y: nums[1], Z: nums[2]}, nums[3], nil
}
