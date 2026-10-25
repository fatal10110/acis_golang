package catalog

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/boss/sailren"
)

// aiBossSailren is the catalog of the Sailren behaviors (#203).
func aiBossSailren() script.Catalog {
	return script.Catalog{
		"script.ai.boss.sailren.SailrenDummy":   sailren.SailrenDummy,
		"script.ai.boss.sailren.StatueOfShilen": sailren.StatueOfShilen,
	}
}
