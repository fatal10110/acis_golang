package catalog

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/boss/core"
)

// aiBossCore is the catalog of the Core behaviors (#200).
func aiBossCore() script.Catalog {
	return script.Catalog{
		"script.ai.boss.core.Core": core.Core,
	}
}
