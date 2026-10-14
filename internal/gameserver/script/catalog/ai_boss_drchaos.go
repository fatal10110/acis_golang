package catalog

import "github.com/fatal10110/acis_golang/internal/gameserver/script"

// aiBossDrChaos is the catalog of the Dr. Chaos behaviors (#205).
func aiBossDrChaos() script.Catalog {
	return script.Catalog{
		"script.ai.boss.drchaos.DoctorChaosNpc": nil,
	}
}
