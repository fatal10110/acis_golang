// Package residence contains XML-backed static residence data shared by
// castles and clan halls.
package residence

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// SpawnType classifies one residence respawn list.
type SpawnType uint8

const (
	SpawnOwner SpawnType = iota
	SpawnOther
	SpawnChaotic
	SpawnBanish
)

var SpawnTypeNames = map[string]SpawnType{
	"OWNER":   SpawnOwner,
	"OTHER":   SpawnOther,
	"CHAOTIC": SpawnChaotic,
	"BANISH":  SpawnBanish,
}

var spawnTypeStrings = [...]string{"OWNER", "OTHER", "CHAOTIC", "BANISH"}

// String returns the canonical XML spelling for s.
func (s SpawnType) String() string {
	if int(s) < len(spawnTypeStrings) {
		return spawnTypeStrings[s]
	}
	return fmt.Sprintf("SpawnType(%d)", uint8(s))
}

// ZoneType classifies one residence polygon.
type ZoneType uint8

const (
	ZoneResidence ZoneType = iota
	ZoneBattlefield
	ZoneHeadquarter
)

var ZoneTypeNames = map[string]ZoneType{
	"RESIDENCE":   ZoneResidence,
	"BATTLEFIELD": ZoneBattlefield,
	"HEADQUARTER": ZoneHeadquarter,
}

var zoneTypeStrings = [...]string{"RESIDENCE", "BATTLEFIELD", "HEADQUARTER"}

// String returns the canonical XML spelling for z.
func (z ZoneType) String() string {
	if int(z) < len(zoneTypeStrings) {
		return zoneTypeStrings[z]
	}
	return fmt.Sprintf("ZoneType(%d)", uint8(z))
}

// Tax stores the static tax settings attached to one residence.
type Tax struct {
	Rate        int
	SysgetRate  int
	TributeRate int
}

// Zone is one polygon entry from a residence XML file.
type Zone struct {
	Type       ZoneType
	MinZ, MaxZ int
	Nodes      []location.Point
}
