package catalog

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/teleport/castledefendteleporter"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/teleport/charmteleporter"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/teleport/dimensionalvortex"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/teleport/elmoredencemeteryteleporter"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/teleport/gatekeeperspirit"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/teleport/huntinggroundsteleporter"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/teleport/monsterracetrack"
)

// teleporters is the catalog of the scripted teleporters (#210), except the two the engine slices ported.
func teleporters() script.Catalog {
	return script.Catalog{
		"script.teleport.CastleDefendTeleporter":      castledefendteleporter.New,
		"script.teleport.CharmTeleporter":             charmteleporter.New,
		"script.teleport.DimensionalVortex":           dimensionalvortex.New,
		"script.teleport.ElmoredenCemeteryTeleporter": elmoredencemeteryteleporter.New,
		"script.teleport.ElrokiTeleporter":            nil,
		"script.teleport.GatekeeperSpirit":            gatekeeperspirit.New,
		"script.teleport.GrandBossTeleporter":         nil,
		"script.teleport.HuntingGroundsTeleporter":    huntinggroundsteleporter.New,
		"script.teleport.MonsterRaceTrack":            monsterracetrack.New,
		"script.teleport.OracleTeleporter":            nil,
		"script.teleport.PaganTeleporter":             nil,
	}
}
