package catalog

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/feature/echocrystal"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/feature/firstclasschange"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/feature/herocirclet"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/feature/heroweapon"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/feature/missqueen"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/feature/raidbossinfo"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/feature/secondclasschange"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/feature/shadowweapon"
)

// features is the catalog of the scripted features (#209), except the six the engine slices ported.
func features() script.Catalog {
	return script.Catalog{
		"script.feature.EchoCrystal":         echocrystal.New,
		"script.feature.FirstClassChange":    firstclasschange.New,
		"script.feature.HeroCirclet":         herocirclet.New,
		"script.feature.HeroWeapon":          heroweapon.New,
		"script.feature.KetraOrcSupport":     nil,
		"script.feature.MissQueen":           missqueen.New,
		"script.feature.RaidbossInfo":        raidbossinfo.New,
		"script.feature.SecondClassChange":   secondclasschange.New,
		"script.feature.ShadowWeapon":        shadowweapon.New,
		"script.feature.VarkaSilenosSupport": nil,
	}
}
