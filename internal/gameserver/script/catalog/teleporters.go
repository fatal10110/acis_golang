package catalog

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/teleport/castledefendteleporter"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/teleport/charmteleporter"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/teleport/dimensionalvortex"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/teleport/elmoredencemeteryteleporter"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/teleport/elrokiteleporter"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/teleport/gatekeeperspirit"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/teleport/grandbossteleporter"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/teleport/huntinggroundsteleporter"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/teleport/monsterracetrack"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/teleport/oracleteleporter"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/teleport/paganteleporter"
)

// teleporters is the catalog of the scripted teleporters (#210), except the two the engine slices ported.
func teleporters() script.Catalog {
	return script.Catalog{
		"script.teleport.CastleDefendTeleporter":      castledefendteleporter.New,
		"script.teleport.CharmTeleporter":             charmteleporter.New,
		"script.teleport.DimensionalVortex":           dimensionalvortex.New,
		"script.teleport.ElmoredenCemeteryTeleporter": elmoredencemeteryteleporter.New,
		"script.teleport.ElrokiTeleporter":            elrokiteleporter.New,
		"script.teleport.GatekeeperSpirit":            gatekeeperspirit.New,
		"script.teleport.GrandBossTeleporter":         grandbossteleporter.New,
		"script.teleport.HuntingGroundsTeleporter":    huntinggroundsteleporter.New,
		"script.teleport.MonsterRaceTrack":            monsterracetrack.New,
		"script.teleport.OracleTeleporter":            oracleteleporter.New,
		"script.teleport.PaganTeleporter":             paganteleporter.New,
	}
}
