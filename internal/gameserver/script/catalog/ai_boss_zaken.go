package catalog

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/boss/zaken"
)

// aiBossZaken is the catalog of the Zaken behaviors (#199).
func aiBossZaken() script.Catalog {
	return script.Catalog{
		"script.ai.boss.zaken.Zaken": zaken.Zaken,
	}
}
