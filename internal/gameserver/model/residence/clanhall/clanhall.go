// Package clanhall contains static clan hall data loaded from clanHalls.xml
// and clanHallDeco.xml.
package clanhall

import (
	"fmt"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/residence"
)

const (
	FuncRestoreHP    = 1
	FuncRestoreMP    = 2
	FuncRestoreExp   = 4
	FuncTeleport     = 5
	FuncDecoCurtains = 7
	FuncSupportMagic = 9
	FuncDecoFixtures = 11
	FuncCreateItem   = 12
)

// Hall is one static clan hall definition from clanHalls.xml.
type Hall struct {
	ID       int
	ParentID int
	Alias    string
	Name     string

	Description string
	Town        string

	AuctionMin int
	Deposit    int
	Lease      int
	Size       int
	Grade      int

	SiegeLength    int64
	ScheduleConfig []int

	Tax    residence.Tax
	Gates  []string
	NPCs   []int
	Spawns map[residence.SpawnType][]location.Location
	Zones  []residence.Zone
}

// IsSiegable reports whether the hall carries siege timing data.
func (h Hall) IsSiegable() bool {
	return h.SiegeLength > 0
}

// HallAttrs holds a Hall's own decoded XML attributes, separate from its
// already-decoded child collections (zones, spawns) passed alongside to
// NewHall.
type HallAttrs struct {
	ID, ParentID                            int
	Alias, Name, Description, Town          string
	AuctionMin, Deposit, Lease, Size, Grade int
	SiegeLength                             int64
	ScheduleConfig                          []int
	Tax                                     residence.Tax
	Gates                                   []string
	NPCs                                    []int
}

// NewHall builds a Hall from its decoded attrs plus already-decoded child
// data.
func NewHall(attrs HallAttrs, zones []residence.Zone, spawns map[residence.SpawnType][]location.Location) (*Hall, error) {
	if attrs.Alias == "" {
		return nil, fmt.Errorf("clanhall %d: alias is required", attrs.ID)
	}
	if attrs.Name == "" {
		return nil, fmt.Errorf("clanhall %d: name is required", attrs.ID)
	}
	if attrs.Description == "" {
		return nil, fmt.Errorf("clanhall %d: desc is required", attrs.ID)
	}
	if attrs.Town == "" {
		return nil, fmt.Errorf("clanhall %d: loc is required", attrs.ID)
	}

	return &Hall{
		ID:             attrs.ID,
		ParentID:       attrs.ParentID,
		Alias:          attrs.Alias,
		Name:           attrs.Name,
		Description:    attrs.Description,
		Town:           attrs.Town,
		AuctionMin:     attrs.AuctionMin,
		Deposit:        attrs.Deposit,
		Lease:          attrs.Lease,
		Size:           attrs.Size,
		Grade:          attrs.Grade,
		SiegeLength:    attrs.SiegeLength,
		ScheduleConfig: append([]int(nil), attrs.ScheduleConfig...),
		Tax:            attrs.Tax,
		Gates:          attrs.Gates,
		NPCs:           append([]int(nil), attrs.NPCs...),
		Spawns:         residence.CopySpawns(spawns),
		Zones:          append([]residence.Zone(nil), zones...),
	}, nil
}

// Table stores clan halls keyed by id and alias.
type Table struct {
	byID    map[int]*Hall
	byAlias map[string]*Hall
	order   []*Hall
}

// NewTable builds a clan hall table and rejects duplicate ids or aliases.
func NewTable(halls []*Hall) (*Table, error) {
	t := &Table{
		byID:    make(map[int]*Hall, len(halls)),
		byAlias: make(map[string]*Hall, len(halls)),
		order:   make([]*Hall, 0, len(halls)),
	}
	for _, entry := range halls {
		if entry == nil {
			return nil, fmt.Errorf("clanhall: nil entry")
		}
		if _, exists := t.byID[entry.ID]; exists {
			return nil, fmt.Errorf("clanhall: duplicate id %d", entry.ID)
		}
		aliasKey := strings.ToLower(entry.Alias)
		if _, exists := t.byAlias[aliasKey]; exists {
			return nil, fmt.Errorf("clanhall: duplicate alias %q", entry.Alias)
		}
		t.byID[entry.ID] = entry
		t.byAlias[aliasKey] = entry
		t.order = append(t.order, entry)
	}
	return t, nil
}

// Len returns the number of loaded halls.
func (t *Table) Len() int {
	if t == nil {
		return 0
	}
	return len(t.order)
}

// Get returns the hall with id.
func (t *Table) Get(id int) (*Hall, bool) {
	if t == nil {
		return nil, false
	}
	entry, ok := t.byID[id]
	return entry, ok
}

// ByAlias returns the hall with alias, case-insensitively.
func (t *Table) ByAlias(alias string) (*Hall, bool) {
	if t == nil {
		return nil, false
	}
	entry, ok := t.byAlias[strings.ToLower(alias)]
	return entry, ok
}

// All returns the loaded halls in file order.
func (t *Table) All() []*Hall {
	if t == nil {
		return nil
	}
	return append([]*Hall(nil), t.order...)
}

// Deco is one decoration level entry from clanHallDeco.xml.
type Deco struct {
	Name  string
	Type  int
	Level int
	Depth int
	Days  int
	Price int
}

// NewDeco builds a Deco from its already-decoded attributes.
func NewDeco(name string, decoType, level, depth, days, price int) (Deco, error) {
	if name == "" {
		return Deco{}, fmt.Errorf("clanhall: deco: name is required")
	}
	return Deco{
		Name:  name,
		Type:  decoType,
		Level: level,
		Depth: depth,
		Days:  days,
		Price: price,
	}, nil
}

// DecoTable stores clan hall decorations keyed by (type, level).
type DecoTable struct {
	order []Deco
	byKey map[[2]int]Deco
}

// NewDecoTable builds a decoration table and rejects duplicate type/level rows.
func NewDecoTable(decos []Deco) (*DecoTable, error) {
	t := &DecoTable{
		order: append([]Deco(nil), decos...),
		byKey: make(map[[2]int]Deco, len(decos)),
	}
	for _, deco := range decos {
		key := [2]int{deco.Type, deco.Level}
		if _, exists := t.byKey[key]; exists {
			return nil, fmt.Errorf("clanhall: duplicate deco type %d level %d", deco.Type, deco.Level)
		}
		t.byKey[key] = deco
	}
	return t, nil
}

// Count returns the number of loaded decoration rows.
func (t *DecoTable) Count() int {
	if t == nil {
		return 0
	}
	return len(t.order)
}

// Get returns the decoration for type/level.
func (t *DecoTable) Get(decoType, level int) (Deco, bool) {
	if t == nil {
		return Deco{}, false
	}
	deco, ok := t.byKey[[2]int{decoType, level}]
	return deco, ok
}

// Fee returns the configured price for type/level, or 0 when absent.
func (t *DecoTable) Fee(decoType, level int) int {
	if deco, ok := t.Get(decoType, level); ok {
		return deco.Price
	}
	return 0
}

// Days returns the configured rental days for type/level, or 0 when absent.
func (t *DecoTable) Days(decoType, level int) int {
	if deco, ok := t.Get(decoType, level); ok {
		return deco.Days
	}
	return 0
}

// Depth returns the configured depth for type/level, or 0 when absent.
func (t *DecoTable) Depth(decoType, level int) int {
	if deco, ok := t.Get(decoType, level); ok {
		return deco.Depth
	}
	return 0
}
