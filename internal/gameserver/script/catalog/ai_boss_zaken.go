package catalog

import "github.com/fatal10110/acis_golang/internal/gameserver/script"

// aiBossZaken is the catalog of the Zaken behaviors (#199).
func aiBossZaken() script.Catalog {
	return script.Catalog{
		"script.ai.boss.zaken.Zaken": nil,
	}
}
