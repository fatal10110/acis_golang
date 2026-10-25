package catalog

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/boss/orfen"
)

// aiBossOrfen is the catalog of the Orfen behaviors (#201).
func aiBossOrfen() script.Catalog {
	return script.Catalog{
		"script.ai.boss.orfen.Orfen":                orfen.Orfen,
		"script.ai.boss.orfen.OrfenPrivateRibaIren": orfen.RibaIren,
	}
}
