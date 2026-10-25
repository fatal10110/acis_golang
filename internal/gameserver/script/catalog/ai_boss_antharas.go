package catalog

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/boss/antharas"
)

// aiBossAntharas is the catalog of the Antharas behaviors (#195).
func aiBossAntharas() script.Catalog {
	return script.Catalog{
		"script.ai.boss.antharas.Antharas":       antharas.Antharas,
		"script.ai.boss.antharas.HeartOfWarding": nil,
	}
}
