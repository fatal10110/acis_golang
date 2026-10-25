package catalog

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/boss/baium"
)

// aiBossBaium is the catalog of the Baium behaviors (#196).
func aiBossBaium() script.Catalog {
	return script.Catalog{
		"script.ai.boss.baium.Archangel":          nil,
		"script.ai.boss.baium.Baium":              baium.Baium,
		"script.ai.boss.baium.BaiumAngelicVortex": nil,
		"script.ai.boss.baium.BaiumStone":         nil,
	}
}
