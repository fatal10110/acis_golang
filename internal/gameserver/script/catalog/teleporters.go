package catalog

import "github.com/fatal10110/acis_golang/internal/gameserver/script"

// teleporters is the catalog of the scripted teleporters (#210), except the two the engine slices ported.
func teleporters() script.Catalog {
	return script.Catalog{
		"script.teleport.CastleDefendTeleporter":      nil,
		"script.teleport.CharmTeleporter":             nil,
		"script.teleport.DimensionalVortex":           nil,
		"script.teleport.ElmoredenCemeteryTeleporter": nil,
		"script.teleport.ElrokiTeleporter":            nil,
		"script.teleport.GatekeeperSpirit":            nil,
		"script.teleport.GrandBossTeleporter":         nil,
		"script.teleport.HuntingGroundsTeleporter":    nil,
		"script.teleport.MonsterRaceTrack":            nil,
		"script.teleport.OracleTeleporter":            nil,
		"script.teleport.PaganTeleporter":             nil,
	}
}
