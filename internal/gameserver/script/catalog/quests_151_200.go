package catalog

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q151"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q152"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q153"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q154"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q155"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q156"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q157"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q158"
)

// quests151To200 is the catalog of quests 151 to 200 (#181).
func quests151To200() script.Catalog {
	return script.Catalog{
		"quest.Q151_CureForFeverDisease":           q151.New,
		"quest.Q152_ShardsOfGolem":                 q152.New,
		"quest.Q153_DeliverGoods":                  q153.New,
		"quest.Q154_SacrificeToTheSea":             q154.New,
		"quest.Q155_FindSirWindawood":              q155.New,
		"quest.Q156_MillenniumLove":                q156.New,
		"quest.Q157_RecoverSmuggledGoods":          q157.New,
		"quest.Q158_SeedOfEvil":                    q158.New,
		"quest.Q159_ProtectTheWaterSource":         nil,
		"quest.Q160_NerupasRequest":                nil,
		"quest.Q161_FruitOfTheMotherTree":          nil,
		"quest.Q162_CurseOfTheUndergroundFortress": nil,
		"quest.Q163_LegacyOfThePoet":               nil,
		"quest.Q164_BloodFiend":                    nil,
		"quest.Q165_ShilensHunt":                   nil,
		"quest.Q166_MassOfDarkness":                nil,
		"quest.Q167_DwarvenKinship":                nil,
		"quest.Q168_DeliverSupplies":               nil,
		"quest.Q169_OffspringOfNightmares":         nil,
		"quest.Q170_DangerousSeduction":            nil,
		"quest.Q171_ActsOfEvil":                    nil,
	}
}
