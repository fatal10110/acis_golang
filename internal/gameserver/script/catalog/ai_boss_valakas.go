package catalog

import "github.com/fatal10110/acis_golang/internal/gameserver/script"

// aiBossValakas is the catalog of the Valakas behaviors (#197).
func aiBossValakas() script.Catalog {
	return script.Catalog{
		"script.ai.boss.valakas.HeartOfVolcano": nil,
		"script.ai.boss.valakas.Valakas":        nil,
	}
}
