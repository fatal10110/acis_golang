package catalog

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/boss/benom"
)

// aiBossBenom is the catalog of the Benom behaviors (#204).
func aiBossBenom() script.Catalog {
	return script.Catalog{
		"script.ai.boss.benom.Benom":             benom.Benom,
		"script.ai.boss.benom.DungeonTeleporter": benom.DungeonTeleporter,
	}
}
