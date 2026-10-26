package catalog

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/boss/sirra"
)

// aiBossSirra is the catalog of the Sirra and Freya behaviors (#206).
func aiBossSirra() script.Catalog {
	return script.Catalog{
		"script.ai.boss.sirra.FreyaDeacon":     sirra.FreyaDeacon,
		"script.ai.boss.sirra.SculptureGarden": sirra.SculptureGarden,
	}
}
