package catalog

import "github.com/fatal10110/acis_golang/internal/gameserver/script"

// aiBossAntharas is the catalog of the Antharas behaviors (#195).
func aiBossAntharas() script.Catalog {
	return script.Catalog{
		"script.ai.boss.antharas.Antharas":       nil,
		"script.ai.boss.antharas.HeartOfWarding": nil,
	}
}
