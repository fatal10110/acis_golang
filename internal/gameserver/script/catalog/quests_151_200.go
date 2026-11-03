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
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q159"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q160"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q161"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q162"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q163"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q164"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q165"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q166"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q167"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q168"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q169"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q170"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q171"
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
		"quest.Q159_ProtectTheWaterSource":         q159.New,
		"quest.Q160_NerupasRequest":                q160.New,
		"quest.Q161_FruitOfTheMotherTree":          q161.New,
		"quest.Q162_CurseOfTheUndergroundFortress": q162.New,
		"quest.Q163_LegacyOfThePoet":               q163.New,
		"quest.Q164_BloodFiend":                    q164.New,
		"quest.Q165_ShilensHunt":                   q165.New,
		"quest.Q166_MassOfDarkness":                q166.New,
		"quest.Q167_DwarvenKinship":                q167.New,
		"quest.Q168_DeliverSupplies":               q168.New,
		"quest.Q169_OffspringOfNightmares":         q169.New,
		"quest.Q170_DangerousSeduction":            q170.New,
		"quest.Q171_ActsOfEvil":                    q171.New,
	}
}
