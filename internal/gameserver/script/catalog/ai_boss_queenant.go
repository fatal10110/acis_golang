package catalog

import "github.com/fatal10110/acis_golang/internal/gameserver/script"

// aiBossQueenAnt is the catalog of the Queen Ant behaviors (#202).
func aiBossQueenAnt() script.Catalog {
	return script.Catalog{
		"script.ai.boss.queenant.QueenAnt":                nil,
		"script.ai.boss.queenant.QueenAntPrivateAntLarva": nil,
		"script.ai.boss.queenant.QueenAntPrivateNurseAnt": nil,
	}
}
