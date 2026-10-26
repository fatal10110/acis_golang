package catalog

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/hall"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/siegablehall/banditstronghold"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/siegablehall/devastatedcastle"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/siegablehall/fortressofresistance"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/siegablehall/fortressofthedead"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/siegablehall/rainbowspringschateau"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/siegablehall/wildbeastreserve"
)

// siegableHall is the catalog of the siegable clan hall scripts and behaviors (#3502).
func siegableHall() script.Catalog {
	watering := hall.NewWateringGame()
	return script.Catalog{
		"script.ai.siegablehall.Agit01Move1.Agit01Move1":                                hall.Move1,
		"script.ai.siegablehall.Agit01Move1.Agit01PartisanSorcerer2":                    hall.PartisanSorcerer2,
		"script.ai.siegablehall.Agit01PartisanHealer1.Agit01PartisanHealer1":            hall.PartisanHealer1,
		"script.ai.siegablehall.Agit01PartisanHealer1.Agit01PartisanHealer2":            hall.PartisanHealer2,
		"script.ai.siegablehall.Agit01PartisanSorcerer1":                                hall.PartisanSorcerer1,
		"script.ai.siegablehall.Agit01Stand1.Agit01Stand1":                              hall.Stand1,
		"script.ai.siegablehall.Agit01Stand1.Agit01Stand2":                              hall.Stand2,
		"script.ai.siegablehall.Agit02Dietrich":                                         hall.Dietrich,
		"script.ai.siegablehall.Agit02Gustav":                                           hall.Gustav,
		"script.ai.siegablehall.Agit02Mikhail":                                          hall.Mikhail,
		"script.ai.siegablehall.Agit03Alfred":                                           hall.Alfred,
		"script.ai.siegablehall.Agit03Giselle":                                          hall.Giselle,
		"script.ai.siegablehall.Agit03Lidia":                                            hall.Lidia,
		"script.ai.siegablehall.AzitWateringCrazyYeti":                                  hall.CrazyYeti,
		"script.ai.siegablehall.AzitWateringGameManager.AzitWateringGameManager":        func() script.Script { return hall.GameManager(watering) },
		"script.ai.siegablehall.AzitWateringGameManager.AzitWateringGameManagerCircle1": func() script.Script { return hall.Circle1(watering) },
		"script.ai.siegablehall.AzitWateringGameManager.AzitWateringGameManagerCircle2": func() script.Script { return hall.Circle2(watering) },
		"script.ai.siegablehall.AzitWateringGameManager.AzitWateringGameManagerCircle3": func() script.Script { return hall.Circle3(watering) },
		"script.ai.siegablehall.AzitWateringGameManager.AzitWateringGameManagerCircle4": func() script.Script { return hall.Circle4(watering) },
		"script.ai.siegablehall.AzitWateringGroud":                                      hall.Gourd,
		"script.ai.siegablehall.AzitWateringMimic":                                      hall.Mimic,
		"script.ai.siegablehall.AzitWateringMimicMagical":                               hall.MimicMagical,
		"script.ai.siegablehall.AzitWateringRecycleCan":                                 hall.RecycleCan,
		"script.ai.siegablehall.AzitWateringTreasureBox":                                hall.TreasureBox,
		"script.ai.siegablehall.BloodyLordNurka1.BloodyLordNurka1":                      hall.Nurka1,
		"script.ai.siegablehall.BloodyLordNurka1.BloodyLordNurka2":                      hall.Nurka2,
		"script.siegablehall.BanditStronghold":                                          banditstronghold.New,
		"script.siegablehall.DevastatedCastle":                                          devastatedcastle.New,
		"script.siegablehall.FortressOfResistance":                                      fortressofresistance.New,
		"script.siegablehall.FortressOfTheDead":                                         fortressofthedead.New,
		"script.siegablehall.RainbowSpringsChateau":                                     rainbowspringschateau.New,
		"script.siegablehall.WildBeastReserve":                                          wildbeastreserve.New,
	}
}
