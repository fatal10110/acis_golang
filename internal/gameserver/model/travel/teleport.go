package travel

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// Kind classifies a gatekeeper teleport option.
type Kind uint8

const (
	KindStandard Kind = iota
	KindNewbieToken
	KindNobleHuntingZonePass
	KindNobleHuntingZoneAdena
	KindAlly
	KindClanHallFunctionLevel1
	KindClanHallFunctionLevel2
)

var kindNames = map[string]Kind{
	"STANDARD":                 KindStandard,
	"NEWBIE_TOKEN":             KindNewbieToken,
	"NOBLE_HUNTING_ZONE_PASS":  KindNobleHuntingZonePass,
	"NOBLE_HUNTING_ZONE_ADENA": KindNobleHuntingZoneAdena,
	"ALLY":                     KindAlly,
	"CHF_LEVEL_1":              KindClanHallFunctionLevel1,
	"CHF_LEVEL_2":              KindClanHallFunctionLevel2,
}

var kindStrings = [...]string{
	"STANDARD", "NEWBIE_TOKEN", "NOBLE_HUNTING_ZONE_PASS",
	"NOBLE_HUNTING_ZONE_ADENA", "ALLY", "CHF_LEVEL_1", "CHF_LEVEL_2",
}

// String returns k's canonical XML spelling.
func (k Kind) String() string {
	if int(k) < len(kindStrings) {
		return kindStrings[k]
	}
	return fmt.Sprintf("Kind(%d)", uint8(k))
}

// Teleport is one gatekeeper destination from teleports.xml.
type Teleport struct {
	location.Location
	Description string
	Kind        Kind
	PriceID     int
	PriceCount  int
	CastleID    int
}

// NewTeleport builds a Teleport from set. desc, priceId, priceCount, x, y,
// and z are required; type defaults to STANDARD and castleId defaults to 0.
func NewTeleport(set *commons.StatSet) (Teleport, error) {
	loc, err := location.NewLocation(set)
	if err != nil {
		return Teleport{}, fmt.Errorf("travel: teleport: %w", err)
	}
	desc, err := set.GetString("desc")
	if err != nil {
		return Teleport{}, fmt.Errorf("travel: teleport: %w", err)
	}
	kind, err := commons.GetEnumDefault(set, "type", kindNames, KindStandard)
	if err != nil {
		return Teleport{}, fmt.Errorf("travel: teleport %q: %w", desc, err)
	}
	priceID, err := set.GetInt("priceId")
	if err != nil {
		return Teleport{}, fmt.Errorf("travel: teleport %q: %w", desc, err)
	}
	priceCount, err := set.GetInt("priceCount")
	if err != nil {
		return Teleport{}, fmt.Errorf("travel: teleport %q: %w", desc, err)
	}
	castleID, err := set.GetIntDefault("castleId", 0)
	if err != nil {
		return Teleport{}, fmt.Errorf("travel: teleport %q: %w", desc, err)
	}
	return Teleport{
		Location:    loc,
		Description: desc,
		Kind:        kind,
		PriceID:     priceID,
		PriceCount:  priceCount,
		CastleID:    castleID,
	}, nil
}

// TeleportTable stores regular gatekeeper destinations keyed by npc id.
type TeleportTable map[int][]Teleport

// Count returns the total number of destinations in the table.
func (t TeleportTable) Count() int {
	var n int
	for _, locs := range t {
		n += len(locs)
	}
	return n
}

// InstantTable stores instant teleport destinations keyed by npc id.
type InstantTable map[int][]location.Location

// Count returns the total number of destinations in the table.
func (t InstantTable) Count() int {
	var n int
	for _, locs := range t {
		n += len(locs)
	}
	return n
}
