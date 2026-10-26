package catalog

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/boss/valakas"
)

// aiBossValakas is the catalog of the Valakas behaviors (#197).
func aiBossValakas() script.Catalog {
	return script.Catalog{
		"script.ai.boss.valakas.HeartOfVolcano": nil,
		"script.ai.boss.valakas.Valakas":        valakas.Valakas,
	}
}
