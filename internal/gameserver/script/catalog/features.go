package catalog

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/feature/firstclasschange"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/feature/secondclasschange"
)

// features is the catalog of the scripted features (#209), except the six the engine slices ported.
func features() script.Catalog {
	return script.Catalog{
		"script.feature.EchoCrystal":         nil,
		"script.feature.FirstClassChange":    firstclasschange.New,
		"script.feature.HeroCirclet":         nil,
		"script.feature.HeroWeapon":          nil,
		"script.feature.KetraOrcSupport":     nil,
		"script.feature.MissQueen":           nil,
		"script.feature.RaidbossInfo":        nil,
		"script.feature.SecondClassChange":   secondclasschange.New,
		"script.feature.ShadowWeapon":        nil,
		"script.feature.VarkaSilenosSupport": nil,
	}
}
