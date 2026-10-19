package catalog

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/hall"
)

// siegableHall is the catalog of the siegable clan hall scripts and behaviors (#3502).
func siegableHall() script.Catalog {
	return script.Catalog{
		"script.ai.siegablehall.Agit01Move1.Agit01Move1":                                nil,
		"script.ai.siegablehall.Agit01Move1.Agit01PartisanSorcerer2":                    nil,
		"script.ai.siegablehall.Agit01PartisanHealer1.Agit01PartisanHealer1":            nil,
		"script.ai.siegablehall.Agit01PartisanHealer1.Agit01PartisanHealer2":            nil,
		"script.ai.siegablehall.Agit01PartisanSorcerer1":                                hall.PartisanSorcerer1,
		"script.ai.siegablehall.Agit01Stand1.Agit01Stand1":                              nil,
		"script.ai.siegablehall.Agit01Stand1.Agit01Stand2":                              nil,
		"script.ai.siegablehall.Agit02Dietrich":                                         hall.Dietrich,
		"script.ai.siegablehall.Agit02Gustav":                                           hall.Gustav,
		"script.ai.siegablehall.Agit02Mikhail":                                          hall.Mikhail,
		"script.ai.siegablehall.Agit03Alfred":                                           hall.Alfred,
		"script.ai.siegablehall.Agit03Giselle":                                          hall.Giselle,
		"script.ai.siegablehall.Agit03Lidia":                                            hall.Lidia,
		"script.ai.siegablehall.AzitWateringCrazyYeti":                                  nil,
		"script.ai.siegablehall.AzitWateringGameManager.AzitWateringGameManager":        nil,
		"script.ai.siegablehall.AzitWateringGameManager.AzitWateringGameManagerCircle1": nil,
		"script.ai.siegablehall.AzitWateringGameManager.AzitWateringGameManagerCircle2": nil,
		"script.ai.siegablehall.AzitWateringGameManager.AzitWateringGameManagerCircle3": nil,
		"script.ai.siegablehall.AzitWateringGameManager.AzitWateringGameManagerCircle4": nil,
		"script.ai.siegablehall.AzitWateringGroud":                                      nil,
		"script.ai.siegablehall.AzitWateringMimic":                                      nil,
		"script.ai.siegablehall.AzitWateringMimicMagical":                               nil,
		"script.ai.siegablehall.AzitWateringRecycleCan":                                 nil,
		"script.ai.siegablehall.AzitWateringTreasureBox":                                nil,
		"script.ai.siegablehall.BloodyLordNurka1.BloodyLordNurka1":                      nil,
		"script.ai.siegablehall.BloodyLordNurka1.BloodyLordNurka2":                      nil,
		"script.siegablehall.BanditStronghold":                                          nil,
		"script.siegablehall.DevastatedCastle":                                          nil,
		"script.siegablehall.FortressOfResistance":                                      nil,
		"script.siegablehall.FortressOfTheDead":                                         nil,
		"script.siegablehall.RainbowSpringsChateau":                                     nil,
		"script.siegablehall.WildBeastReserve":                                          nil,
	}
}
