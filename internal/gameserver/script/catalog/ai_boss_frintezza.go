package catalog

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/boss/frintezza"
)

// aiBossFrintezza is the catalog of the Frintezza behaviors (#198).
func aiBossFrintezza() script.Catalog {
	return script.Catalog{
		"script.ai.boss.frintezza.AlarmDevice":                  frintezza.AlarmDevice,
		"script.ai.boss.frintezza.EvilateA":                     frintezza.EvilateA,
		"script.ai.boss.frintezza.EvilateB":                     frintezza.EvilateB,
		"script.ai.boss.frintezza.FollowerDummy":                frintezza.FollowerDummy,
		"script.ai.boss.frintezza.FollowerOfFrintezza":          frintezza.FollowerOfFrintezza,
		"script.ai.boss.frintezza.FollowerOfFrintezzaTransform": frintezza.FollowerOfFrintezzaTransform,
		"script.ai.boss.frintezza.Frintezza":                    frintezza.Frintezza,
		"script.ai.boss.frintezza.FrintezzaSeeker":              frintezza.FrintezzaSeeker,
		"script.ai.boss.frintezza.FrintezzaTeleporter":          nil,
		"script.ai.boss.frintezza.OrganDummy":                   frintezza.OrganDummy,
		"script.ai.boss.frintezza.PortraitSpirit":               nil,
		"script.ai.boss.frintezza.PortraitSpiritWinged":         nil,
		"script.ai.boss.frintezza.UndeadBandmaster":             nil,
	}
}
