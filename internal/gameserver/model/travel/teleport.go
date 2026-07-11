package travel

import (
	"fmt"
	"time"

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
	df := commons.NewFields(set, "travel: teleport")
	desc := df.String("desc")
	if err := df.Err(); err != nil {
		return Teleport{}, err
	}

	f := commons.NewFields(set, fmt.Sprintf("travel: teleport %q", desc))
	teleport := Teleport{
		Location:    loc,
		Description: desc,
		Kind:        commons.FieldEnumDefault[Kind](f, "type", kindNames, KindStandard),
		PriceID:     f.Int("priceId"),
		PriceCount:  f.Int("priceCount"),
		CastleID:    f.IntDefault("castleId", 0),
	}
	if err := f.Err(); err != nil {
		return Teleport{}, err
	}
	return teleport, nil
}

// CalculatedPrice returns t's price at the given instant: standard
// destinations are half price (rounded down, minimum 1) during weekend core
// time, 20:00 through 23:59. Any per-player discount tied to event/seal
// state is not applied here and stays the caller's responsibility once that
// state exists.
func (t Teleport) CalculatedPrice(now time.Time) int {
	if t.Kind == KindStandard && isCoreTime(now) {
		return max(t.PriceCount>>1, 1)
	}
	return t.PriceCount
}

// isCoreTime reports whether now falls in weekend core time (Saturday or
// Sunday, 20:00 through 23:59), in now's own location.
func isCoreTime(now time.Time) bool {
	switch now.Weekday() {
	case time.Saturday, time.Sunday:
		return now.Hour() >= 20
	default:
		return false
	}
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
