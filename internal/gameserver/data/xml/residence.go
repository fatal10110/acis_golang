package xml

import (
	"encoding/xml"
	"fmt"
	"strings"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/residence"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/residence/castle"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/residence/clanhall"
)

type castleFile struct {
	Castles []castleElement `xml:"castle"`
}

// castleElement is one <castle>. Its scalar attributes are kept raw and
// folded with every <tax> child's (see residenceAttrs).
type castleElement struct {
	Attrs         []xml.Attr              `xml:",any,attr"`
	Artifacts     []castleArtifactElement `xml:"artifacts>artifact"`
	ControlTowers []castleTowerElement    `xml:"controlTowers>controlTower"`
	Gates         []valListElement        `xml:"gates"`
	NPCs          []valListElement        `xml:"npcs"`
	Spawns        []residenceSpawnElement `xml:"spawns>spawn"`
	Tax           []attrsElement          `xml:"tax"`
	Tickets       []castleTicketElement   `xml:"tickets>ticket"`
	Zones         []residenceZoneElement  `xml:"zones>zone"`
}

type castleArtifactElement struct {
	ID  *literal32 `xml:"id,attr"`
	Pos string     `xml:"pos,attr"`
}

type castleTowerElement struct {
	Alias    string              `xml:"alias,attr"`
	Type     string              `xml:"type,attr"`
	Position []literalLocation   `xml:"position"`
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

// valListElement is a "<tag val=\"a;b;c\"/>" child holding a
// semicolon-delimited list (gates, npcs, a control tower's zone aliases).
type valListElement struct {
	Val string `xml:"val,attr"`
}

type clanHallFile struct {
	Halls []clanHallElement `xml:"clanHall"`
}

// clanHallElement is one <clanHall>. Its scalar attributes are kept raw and
// folded with every <agit> child's, then every <tax> child's (see
// residenceAttrs).
type clanHallElement struct {
	Attrs  []xml.Attr              `xml:",any,attr"`
	Agits  []attrsElement          `xml:"agit"`
	Gates  []valListElement        `xml:"gates"`
	NPCs   []valListElement        `xml:"npcs"`
	Spawns []residenceSpawnElement `xml:"spawns>spawn"`
	Taxes  []attrsElement          `xml:"tax"`
	Zones  []residenceZoneElement  `xml:"zones>zone"`
}

// residenceAttrs is a residence's scalar attribute set: the element's own
// attributes, then each folded child group's in turn, every child in
// document order. A later value replaces an earlier one of the same name and
// a name a later child omits keeps its earlier value, so any of the folded
// elements may supply any key. Values stay raw until read, so only the final
// value of a key is parsed.
type residenceAttrs map[string]string

func foldResidenceAttrs(own []xml.Attr, groups ...[]attrsElement) residenceAttrs {
	out := make(residenceAttrs, len(own))
	for _, a := range own {
		out[a.Name.Local] = a.Value
	}
	for _, group := range groups {
		for _, child := range group {
			for _, a := range child.Attrs {
				out[a.Name.Local] = a.Value
			}
		}
	}
	return out
}

// requiredInt reads a required int with the coord grammar (commons.Atoi).
func (r residenceAttrs) requiredInt(key string) (int, error) {
	raw, ok := r[key]
	if !ok {
		return 0, fmt.Errorf("%s is required", key)
	}
	n, err := commons.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	return n, nil
}

// optionalInt64 reads an optional int64, reporting whether key is present.
func (r residenceAttrs) optionalInt64(key string) (int64, bool, error) {
	raw, ok := r[key]
	if !ok {
		return 0, false, nil
	}
	n, err := commons.ParseInt(raw, 64)
	if err != nil {
		return 0, true, fmt.Errorf("%s: %w", key, err)
	}
	return n, true, nil
}

// optionalInt reads an int that defaults to 0 when absent; a present
// malformed value is an error.
func (r residenceAttrs) optionalInt(key string) (int, error) {
	if _, ok := r[key]; !ok {
		return 0, nil
	}
	return r.requiredInt(key)
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

// residenceZoneElement is a residence's <zone> outline. Its integers follow
// the zone files' grammar (integer literals) for the bounds and the nodes
// alike.
type residenceZoneElement struct {
	Type  string         `xml:"type,attr"`
	MinZ  *literal32     `xml:"minZ,attr"`
	MaxZ  *literal32     `xml:"maxZ,attr"`
	Nodes []pointElement `xml:"node"`
}

// residenceSpawnElement is one <spawn> under a castle or clan hall: a spawn
// kind plus the coordinates it applies to.
type residenceSpawnElement struct {
	Type string `xml:"type,attr"`
	literalLocation
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
	if el.Depth == nil || el.Days == nil || el.Price == nil {
		return clanhall.Deco{}, fmt.Errorf("clanhall: deco %q: depth, days and price are required", el.Name)
	}
	var decoType, level int
	if el.Type != nil {
		decoType = int(*el.Type)
	}
	if el.Level != nil {
		level = int(*el.Level)
	}
	return clanhall.NewDeco(el.Name, decoType, level, int(*el.Depth), int(*el.Days), int(*el.Price))
}

func buildCastle(el castleElement) (*castle.Castle, error) {
	attrs := foldResidenceAttrs(el.Attrs, el.Tax)
	id, err := attrs.requiredInt("id")
	if err != nil {
		return nil, fmt.Errorf("castle: %w", err)
	}
	parentID, err := attrs.requiredInt("parentId")
	if err != nil {
		return nil, fmt.Errorf("castle %d: %w", id, err)
	}
	circletID, err := attrs.requiredInt("circletId")
	if err != nil {
		return nil, fmt.Errorf("castle %d: %w", id, err)
	}

	tax, err := buildResidenceTax(attrs)
	if err != nil {
		return nil, fmt.Errorf("castle %d: %w", id, err)
	}

	var npcs []int
	if npcsRaw := joinVals(el.NPCs); npcsRaw != "" {
		var err error
		npcs, err = splitInts(npcsRaw)
		if err != nil {
			return nil, fmt.Errorf("castle %d: npcs: %w", id, err)
		}
	}
	gates := cleanStrings(splitList(joinVals(el.Gates)))

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
		ParentID:  parentID,
		CircletID: circletID,
		Alias:     attrs["alias"],
		Name:      attrs["name"],
		Tax:       tax,
		Gates:     gates,
		NPCs:      npcs,
	}, artifacts, towers, tickets, zones, spawns)
}

func buildControlTower(t castleTowerElement) (castle.ControlTower, error) {
	if len(t.Position) == 0 {
		return castle.ControlTower{}, fmt.Errorf("castle: control tower %q: position is required", t.Alias)
	}
	// Last element wins: the StatSet this replaced was built by merging every
	// <position>/<stats> child in document order, so a later child's attrs
	// overwrote an earlier one's.
	loc, err := t.Position[len(t.Position)-1].loc()
	if err != nil {
		return castle.ControlTower{}, fmt.Errorf("castle: control tower %q: %w", t.Alias, err)
	}
	if len(t.Stats) == 0 {
		return castle.ControlTower{}, fmt.Errorf("castle: control tower %q: stats is required", t.Alias)
	}
	stats := t.Stats[len(t.Stats)-1]
	if stats.HP == nil || stats.PDef == nil || stats.MDef == nil {
		return castle.ControlTower{}, fmt.Errorf("castle: control tower %q: stats hp, pDef and mDef are required", t.Alias)
	}
	// Last <zones> child wins, like <position> and <stats>.
	var zoneList string
	if len(t.Zones) > 0 {
		zoneList = t.Zones[len(t.Zones)-1].Val
	}
	zones := cleanStrings(splitList(zoneList))
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
	attrs := foldResidenceAttrs(el.Attrs, el.Agits, el.Taxes)
	id, err := attrs.requiredInt("id")
	if err != nil {
		return nil, fmt.Errorf("clanhall: %w", err)
	}
	parentID, err := attrs.requiredInt("parentId")
	if err != nil {
		return nil, fmt.Errorf("clanhall %d: %w", id, err)
	}

	// auctionMin, deposit, lease, size and grade default to 0 when absent.
	var fees [5]int
	for i, key := range [...]string{"auctionMin", "deposit", "lease", "size", "grade"} {
		if fees[i], err = attrs.optionalInt(key); err != nil {
			return nil, fmt.Errorf("clanhall %d: %w", id, err)
		}
	}

	// A siegeLength key makes the hall siegable; only then is scheduleConfig
	// read, and it must hold at least one int.
	siegeLength, siegable, err := attrs.optionalInt64("siegeLength")
	if err != nil {
		return nil, fmt.Errorf("clanhall %d: %w", id, err)
	}
	var scheduleConfig []int
	if siegable {
		raw := attrs["scheduleConfig"]
		if raw == "" {
			return nil, fmt.Errorf("clanhall %d: scheduleConfig is required for a siegable hall", id)
		}
		if scheduleConfig, err = splitInts(raw); err != nil {
			return nil, fmt.Errorf("clanhall %d: scheduleConfig: %w", id, err)
		}
	}

	tax, err := buildResidenceTax(attrs)
	if err != nil {
		return nil, fmt.Errorf("clanhall %d: %w", id, err)
	}

	var npcs []int
	if npcsRaw := joinVals(el.NPCs); npcsRaw != "" {
		var err error
		npcs, err = splitInts(npcsRaw)
		if err != nil {
			return nil, fmt.Errorf("clanhall %d: npcs: %w", id, err)
		}
	}
	gates := cleanStrings(splitList(joinVals(el.Gates)))

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
		ParentID:       parentID,
		Alias:          attrs["alias"],
		Name:           attrs["name"],
		Description:    attrs["desc"],
		Town:           attrs["loc"],
		AuctionMin:     fees[0],
		Deposit:        fees[1],
		Lease:          fees[2],
		Size:           fees[3],
		Grade:          fees[4],
		SiegeLength:    siegeLength,
		Siegable:       siegable,
		ScheduleConfig: scheduleConfig,
		Tax:            tax,
		Gates:          gates,
		NPCs:           npcs,
	}, zones, spawns)
}

// buildResidenceTax reads a residence's tax rates from its folded attributes;
// all three are required.
func buildResidenceTax(attrs residenceAttrs) (residence.Tax, error) {
	rate, err := attrs.requiredInt("taxRate")
	if err != nil {
		return residence.Tax{}, err
	}
	sysgetRate, err := attrs.requiredInt("taxSysgetRate")
	if err != nil {
		return residence.Tax{}, err
	}
	tributeRate, err := attrs.requiredInt("tributeRate")
	if err != nil {
		return residence.Tax{}, err
	}
	return residence.Tax{Rate: rate, SysgetRate: sysgetRate, TributeRate: tributeRate}, nil
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

func joinVals(elems []valListElement) string {
	vals := make([]string, 0, len(elems))
	for _, el := range elems {
		vals = append(vals, el.Val)
	}
	return strings.Join(vals, ";")
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
		n, err := commons.Atoi(p)
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
