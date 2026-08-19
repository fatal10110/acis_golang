package xml

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/residence"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/residence/castle"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/residence/clanhall"
)

type castleFile struct {
	Castles []castleElement `xml:"castle"`
}

type castleElement struct {
	ID            *coord                  `xml:"id,attr"`
	Alias         string                  `xml:"alias,attr"`
	ParentID      *coord                  `xml:"parentId,attr"`
	Name          string                  `xml:"name,attr"`
	CircletID     *coord                  `xml:"circletId,attr"`
	Artifacts     []castleArtifactElement `xml:"artifacts>artifact"`
	ControlTowers []castleTowerElement    `xml:"controlTowers>controlTower"`
	Gates         []valListElement        `xml:"gates"`
	NPCs          []valListElement        `xml:"npcs"`
	Spawns        []residenceSpawnElement `xml:"spawns>spawn"`
	Tax           []taxElement            `xml:"tax"`
	Tickets       []castleTicketElement   `xml:"tickets>ticket"`
	Zones         []residenceZoneElement  `xml:"zones>zone"`
}

type castleArtifactElement struct {
	ID  *coord `xml:"id,attr"`
	Pos string `xml:"pos,attr"`
}

type castleTowerElement struct {
	Alias    string              `xml:"alias,attr"`
	Type     string              `xml:"type,attr"`
	Position []locationElement   `xml:"position"`
	Stats    []towerStatsElement `xml:"stats"`
	Zones    []valListElement    `xml:"zones"`
}

type towerStatsElement struct {
	HP   *floatAttr `xml:"hp,attr"`
	PDef *floatAttr `xml:"pDef,attr"`
	MDef *floatAttr `xml:"mDef,attr"`
}

type castleTicketElement struct {
	ItemID     *coord   `xml:"itemId,attr"`
	Type       string   `xml:"type,attr"`
	Stationary boolAttr `xml:"stationary,attr"`
	NPCID      *coord   `xml:"npcId,attr"`
	MaxAmount  *coord   `xml:"maxAmount,attr"`
	SSQ        string   `xml:"ssq,attr"`
}

// taxElement is a residence's <tax> child: the rate attributes read as
// pointers because a missing rate must stay distinguishable from a present
// zero, matching commons.StatSet.GetInt's required-key rejection.
type taxElement struct {
	Rate        *coord `xml:"taxRate,attr"`
	SysgetRate  *coord `xml:"taxSysgetRate,attr"`
	TributeRate *coord `xml:"tributeRate,attr"`
}

// valListElement is a "<tag val=\"a;b;c\"/>" child holding a
// semicolon-delimited list (gates, npcs, a control tower's zone aliases).
type valListElement struct {
	Val string `xml:"val,attr"`
}

type clanHallFile struct {
	Halls []clanHallElement `xml:"clanHall"`
}

type clanHallElement struct {
	ID       *coord                  `xml:"id,attr"`
	Alias    string                  `xml:"alias,attr"`
	ParentID *coord                  `xml:"parentId,attr"`
	Name     string                  `xml:"name,attr"`
	Agits    []agitElement           `xml:"agit"`
	Gates    []valListElement        `xml:"gates"`
	NPCs     []valListElement        `xml:"npcs"`
	Spawns   []residenceSpawnElement `xml:"spawns>spawn"`
	Taxes    []taxElement            `xml:"tax"`
	Zones    []residenceZoneElement  `xml:"zones>zone"`
}

type agitElement struct {
	Desc           string       `xml:"desc,attr"`
	Loc            string       `xml:"loc,attr"`
	SiegeLength    *coord64     `xml:"siegeLength,attr"`
	ScheduleConfig string       `xml:"scheduleConfig,attr"`
	AuctionMin     looseIntAttr `xml:"auctionMin,attr"`
	Deposit        looseIntAttr `xml:"deposit,attr"`
	Lease          looseIntAttr `xml:"lease,attr"`
	Size           looseIntAttr `xml:"size,attr"`
	Grade          looseIntAttr `xml:"grade,attr"`
}

type clanHallDecoFile struct {
	Decos []decoElement `xml:"deco"`
}

type decoElement struct {
	Name  string `xml:"name,attr"`
	Type  *coord `xml:"type,attr"`
	Level *coord `xml:"level,attr"`
	Depth *coord `xml:"depth,attr"`
	Days  *coord `xml:"days,attr"`
	Price *coord `xml:"price,attr"`
}

type residenceZoneElement struct {
	Type  string         `xml:"type,attr"`
	MinZ  *coord         `xml:"minZ,attr"`
	MaxZ  *coord         `xml:"maxZ,attr"`
	Nodes []pointElement `xml:"node"`
}

// residenceSpawnElement is one <spawn> under a castle or clan hall: a spawn
// kind plus the coordinates it applies to.
type residenceSpawnElement struct {
	Type string `xml:"type,attr"`
	locationElement
}

// LoadCastles parses castles.xml into static castle data.
func LoadCastles(path string) (*castle.Table, error) {
	var doc castleFile
	if err := readXML(path, &doc); err != nil {
		return nil, fmt.Errorf("castles: %w", err)
	}

	castles := make([]*castle.Castle, 0, len(doc.Castles))
	for _, el := range doc.Castles {
		entry, err := buildCastle(el)
		if err != nil {
			return nil, fmt.Errorf("xml: %s: %w", path, err)
		}
		castles = append(castles, entry)
	}
	table, err := castle.NewTable(castles)
	if err != nil {
		return nil, fmt.Errorf("xml: %s: %w", path, err)
	}
	return table, nil
}

// LoadClanHalls parses clanHalls.xml into static clan hall data.
func LoadClanHalls(path string) (*clanhall.Table, error) {
	var doc clanHallFile
	if err := readXML(path, &doc); err != nil {
		return nil, fmt.Errorf("clan halls: %w", err)
	}

	halls := make([]*clanhall.Hall, 0, len(doc.Halls))
	for _, el := range doc.Halls {
		entry, err := buildClanHall(el)
		if err != nil {
			return nil, fmt.Errorf("xml: %s: %w", path, err)
		}
		halls = append(halls, entry)
	}
	table, err := clanhall.NewTable(halls)
	if err != nil {
		return nil, fmt.Errorf("xml: %s: %w", path, err)
	}
	return table, nil
}

// LoadClanHallDeco parses clanHallDeco.xml into lookupable decoration data.
func LoadClanHallDeco(path string) (*clanhall.DecoTable, error) {
	var doc clanHallDecoFile
	if err := readXML(path, &doc); err != nil {
		return nil, fmt.Errorf("clan hall deco: %w", err)
	}

	decos := make([]clanhall.Deco, 0, len(doc.Decos))
	for _, el := range doc.Decos {
		entry, err := buildDeco(el)
		if err != nil {
			return nil, fmt.Errorf("xml: %s: %w", path, err)
		}
		decos = append(decos, entry)
	}
	table, err := clanhall.NewDecoTable(decos)
	if err != nil {
		return nil, fmt.Errorf("xml: %s: %w", path, err)
	}
	return table, nil
}

func buildDeco(el decoElement) (clanhall.Deco, error) {
	if el.Name == "" {
		return clanhall.Deco{}, fmt.Errorf("clanhall: deco: name is required")
	}
	if el.Type == nil || el.Level == nil || el.Depth == nil || el.Days == nil || el.Price == nil {
		return clanhall.Deco{}, fmt.Errorf("clanhall: deco %q: type, level, depth, days and price are required", el.Name)
	}
	return clanhall.NewDeco(el.Name, int(*el.Type), int(*el.Level), int(*el.Depth), int(*el.Days), int(*el.Price))
}

func buildCastle(el castleElement) (*castle.Castle, error) {
	if el.ID == nil {
		return nil, fmt.Errorf("castle: id is required")
	}
	id := int(*el.ID)
	if el.ParentID == nil {
		return nil, fmt.Errorf("castle %d: parentId is required", id)
	}
	if el.CircletID == nil {
		return nil, fmt.Errorf("castle %d: circletId is required", id)
	}

	tax, err := buildResidenceTax(el.Tax)
	if err != nil {
		return nil, fmt.Errorf("castle %d: %w", id, err)
	}

	npcsRaw := firstVal(el.NPCs)
	if npcsRaw == "" {
		return nil, fmt.Errorf("castle %d: npcs is required", id)
	}
	npcs, err := splitInts(npcsRaw)
	if err != nil {
		return nil, fmt.Errorf("castle %d: npcs: %w", id, err)
	}
	gates := cleanStrings(splitList(firstVal(el.Gates)))

	artifacts := make([]castle.Artifact, 0, len(el.Artifacts))
	for _, a := range el.Artifacts {
		if a.ID == nil {
			return nil, fmt.Errorf("castle %d: artifact: id is required", id)
		}
		entry, err := castle.NewArtifact(int(*a.ID), a.Pos)
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, entry)
	}

	towers := make([]castle.ControlTower, 0, len(el.ControlTowers))
	for _, t := range el.ControlTowers {
		entry, err := buildControlTower(t)
		if err != nil {
			return nil, err
		}
		towers = append(towers, entry)
	}

	tickets := make([]castle.Ticket, 0, len(el.Tickets))
	for _, tk := range el.Tickets {
		entry, err := buildCastleTicket(tk)
		if err != nil {
			return nil, err
		}
		tickets = append(tickets, entry)
	}

	zones, err := buildResidenceZones(el.Zones)
	if err != nil {
		return nil, err
	}
	spawns, err := buildResidenceSpawns(el.Spawns)
	if err != nil {
		return nil, err
	}

	return castle.NewCastle(castle.CastleAttrs{
		ID:        id,
		ParentID:  int(*el.ParentID),
		CircletID: int(*el.CircletID),
		Alias:     el.Alias,
		Name:      el.Name,
		Tax:       tax,
		Gates:     gates,
		NPCs:      npcs,
	}, artifacts, towers, tickets, zones, spawns)
}

func buildControlTower(t castleTowerElement) (castle.ControlTower, error) {
	if len(t.Position) == 0 {
		return castle.ControlTower{}, fmt.Errorf("castle: control tower %q: position is required", t.Alias)
	}
	loc, err := t.Position[0].loc()
	if err != nil {
		return castle.ControlTower{}, fmt.Errorf("castle: control tower %q: %w", t.Alias, err)
	}
	if len(t.Stats) == 0 {
		return castle.ControlTower{}, fmt.Errorf("castle: control tower %q: stats is required", t.Alias)
	}
	stats := t.Stats[0]
	if stats.HP == nil || stats.PDef == nil || stats.MDef == nil {
		return castle.ControlTower{}, fmt.Errorf("castle: control tower %q: stats hp, pDef and mDef are required", t.Alias)
	}
	zones := cleanStrings(splitList(firstVal(t.Zones)))
	return castle.NewControlTower(t.Alias, t.Type, loc.X, loc.Y, loc.Z, float64(*stats.HP), float64(*stats.PDef), float64(*stats.MDef), zones)
}

func buildCastleTicket(t castleTicketElement) (castle.Ticket, error) {
	if t.ItemID == nil {
		return castle.Ticket{}, fmt.Errorf("castle: ticket: itemId is required")
	}
	if t.NPCID == nil {
		return castle.Ticket{}, fmt.Errorf("castle: ticket %d: npcId is required", *t.ItemID)
	}
	if t.MaxAmount == nil {
		return castle.Ticket{}, fmt.Errorf("castle: ticket %d: maxAmount is required", *t.ItemID)
	}
	ssq := cleanStrings(splitList(t.SSQ))
	return castle.NewTicket(int(*t.ItemID), t.Type, bool(t.Stationary), int(*t.NPCID), int(*t.MaxAmount), ssq)
}

func buildClanHall(el clanHallElement) (*clanhall.Hall, error) {
	if el.ID == nil {
		return nil, fmt.Errorf("clanhall: id is required")
	}
	id := int(*el.ID)
	if el.ParentID == nil {
		return nil, fmt.Errorf("clanhall %d: parentId is required", id)
	}

	if len(el.Agits) == 0 {
		return nil, fmt.Errorf("clanhall %d: agit is required", id)
	}
	agit := el.Agits[0]

	var siegeLength int64
	if agit.SiegeLength != nil {
		siegeLength = int64(*agit.SiegeLength)
	}
	scheduleConfig, err := splitInts(agit.ScheduleConfig)
	if err != nil {
		return nil, fmt.Errorf("clanhall %d: scheduleConfig: %w", id, err)
	}

	tax, err := buildResidenceTax(el.Taxes)
	if err != nil {
		return nil, fmt.Errorf("clanhall %d: %w", id, err)
	}

	npcsRaw := firstVal(el.NPCs)
	if npcsRaw == "" {
		return nil, fmt.Errorf("clanhall %d: npcs is required", id)
	}
	npcs, err := splitInts(npcsRaw)
	if err != nil {
		return nil, fmt.Errorf("clanhall %d: npcs: %w", id, err)
	}
	gates := cleanStrings(splitList(firstVal(el.Gates)))

	zones, err := buildResidenceZones(el.Zones)
	if err != nil {
		return nil, err
	}
	spawns, err := buildResidenceSpawns(el.Spawns)
	if err != nil {
		return nil, err
	}

	return clanhall.NewHall(clanhall.HallAttrs{
		ID:             id,
		ParentID:       int(*el.ParentID),
		Alias:          el.Alias,
		Name:           el.Name,
		Description:    agit.Desc,
		Town:           agit.Loc,
		AuctionMin:     int(agit.AuctionMin),
		Deposit:        int(agit.Deposit),
		Lease:          int(agit.Lease),
		Size:           int(agit.Size),
		Grade:          int(agit.Grade),
		SiegeLength:    siegeLength,
		ScheduleConfig: scheduleConfig,
		Tax:            tax,
		Gates:          gates,
		NPCs:           npcs,
	}, zones, spawns)
}

// buildResidenceTax builds a residence.Tax from a castle's or clan hall's
// <tax> child, using only the first element if more than one is present.
func buildResidenceTax(taxes []taxElement) (residence.Tax, error) {
	if len(taxes) == 0 {
		return residence.Tax{}, fmt.Errorf("tax is required")
	}
	t := taxes[0]
	if t.Rate == nil || t.SysgetRate == nil || t.TributeRate == nil {
		return residence.Tax{}, fmt.Errorf("taxRate, taxSysgetRate and tributeRate are required")
	}
	return residence.Tax{
		Rate:        int(*t.Rate),
		SysgetRate:  int(*t.SysgetRate),
		TributeRate: int(*t.TributeRate),
	}, nil
}

func buildResidenceZones(elems []residenceZoneElement) ([]residence.Zone, error) {
	zones := make([]residence.Zone, 0, len(elems))
	for _, el := range elems {
		kind, ok := residence.ZoneTypeNames[el.Type]
		if !ok {
			return nil, fmt.Errorf("residence: zone: unrecognized type %q", el.Type)
		}
		if el.MinZ == nil || el.MaxZ == nil {
			return nil, fmt.Errorf("residence: zone %q: minZ and maxZ are required", el.Type)
		}
		nodes := make([]location.Point, 0, len(el.Nodes))
		for _, nodeEl := range el.Nodes {
			node, err := nodeEl.point()
			if err != nil {
				return nil, err
			}
			nodes = append(nodes, node)
		}
		zones = append(zones, residence.Zone{
			Type:  kind,
			MinZ:  int(*el.MinZ),
			MaxZ:  int(*el.MaxZ),
			Nodes: nodes,
		})
	}
	return zones, nil
}

func buildResidenceSpawns(elems []residenceSpawnElement) (map[residence.SpawnType][]location.Location, error) {
	if len(elems) == 0 {
		return nil, nil
	}
	out := make(map[residence.SpawnType][]location.Location)
	for _, el := range elems {
		kind, ok := residence.SpawnTypeNames[el.Type]
		if !ok {
			return nil, fmt.Errorf("unknown residence spawn type %q", el.Type)
		}
		loc, err := el.loc()
		if err != nil {
			return nil, err
		}
		out[kind] = append(out[kind], loc)
	}
	return out, nil
}

// firstVal returns the val attribute of the first element in elems, or "" if
// elems is empty. It mirrors the original StatSet-merge behavior of using
// only the first "<tag val=\"...\"/>" child when more than one is present.
func firstVal(elems []valListElement) string {
	if len(elems) == 0 {
		return ""
	}
	return elems[0].Val
}

// splitList splits raw on ";" without trimming or dropping empty elements,
// or returns nil if raw is empty. It mirrors
// commons.StatSet.GetStringArray/GetStringArrayDefault's raw split; callers
// that need trimmed, non-empty elements apply cleanStrings.
func splitList(raw string) []string {
	if raw == "" {
		return nil
	}
	return strings.Split(raw, ";")
}

// splitInts splits raw on ";" and parses each part as an int, or returns nil
// if raw is empty. It mirrors commons.StatSet's coerceIntArray: no
// trimming, and a malformed element is an error.
func splitInts(raw string) ([]int, error) {
	if raw == "" {
		return nil, nil
	}
	parts := strings.Split(raw, ";")
	out := make([]int, len(parts))
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil, err
		}
		out[i] = n
	}
	return out, nil
}

// cleanStrings trims whitespace from each element of in and drops any that
// are empty afterward, or returns nil if nothing remains.
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
