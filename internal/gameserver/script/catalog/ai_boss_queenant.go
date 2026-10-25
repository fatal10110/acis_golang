package catalog

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/boss/queenant"
)

// aiBossQueenAnt is the catalog of the Queen Ant behaviors (#202).
func aiBossQueenAnt() script.Catalog {
	return script.Catalog{
		"script.ai.boss.queenant.QueenAnt":                queenant.QueenAnt,
		"script.ai.boss.queenant.QueenAntPrivateAntLarva": queenant.Larva,
		"script.ai.boss.queenant.QueenAntPrivateNurseAnt": queenant.Nurse,
	}
}
