package catalog

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q002"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q003"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q004"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q005"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q006"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q007"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q008"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q009"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q010"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q011"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q012"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q013"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q014"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q015"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q016"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q017"
)

// quests001To050 is the catalog of quests 001 to 050 (#178), except the two the engine slices ported.
func quests001To050() script.Catalog {
	return script.Catalog{
		"quest.Q002_WhatWomenWant":                    q002.New,
		"quest.Q003_WillTheSealBeBroken":              q003.New,
		"quest.Q004_LongliveThePaagrioLord":           q004.New,
		"quest.Q005_MinersFavor":                      q005.New,
		"quest.Q006_StepIntoTheFuture":                q006.New,
		"quest.Q007_ATripBegins":                      q007.New,
		"quest.Q008_AnAdventureBegins":                q008.New,
		"quest.Q009_IntoTheCityOfHumans":              q009.New,
		"quest.Q010_IntoTheWorld":                     q010.New,
		"quest.Q011_SecretMeetingWithKetraOrcs":       q011.New,
		"quest.Q012_SecretMeetingWithVarkaSilenos":    q012.New,
		"quest.Q013_ParcelDelivery":                   q013.New,
		"quest.Q014_WhereaboutsOfTheArchaeologist":    q014.New,
		"quest.Q015_SweetWhispers":                    q015.New,
		"quest.Q016_TheComingDarkness":                q016.New,
		"quest.Q017_LightAndDarkness":                 q017.New,
		"quest.Q018_MeetingWithTheGoldenRam":          nil,
		"quest.Q019_GoToThePastureland":               nil,
		"quest.Q020_BringUpWithLove":                  nil,
		"quest.Q021_HiddenTruth":                      nil,
		"quest.Q022_TragedyInVonHellmannForest":       nil,
		"quest.Q024_InhabitantsOfTheForestOfTheDead":  nil,
		"quest.Q025_HidingBehindTheTruth":             nil,
		"quest.Q027_ChestCaughtWithABaitOfWind":       nil,
		"quest.Q028_ChestCaughtWithABaitOfIcyAir":     nil,
		"quest.Q029_ChestCaughtWithABaitOfEarth":      nil,
		"quest.Q030_ChestCaughtWithABaitOfFire":       nil,
		"quest.Q031_SecretBuriedInTheSwamp":           nil,
		"quest.Q032_AnObviousLie":                     nil,
		"quest.Q033_MakeAPairOfDressShoes":            nil,
		"quest.Q034_InSearchOfCloth":                  nil,
		"quest.Q035_FindGlitteringJewelry":            nil,
		"quest.Q036_MakeASewingKit":                   nil,
		"quest.Q037_MakeFormalWear":                   nil,
		"quest.Q038_DragonFangs":                      nil,
		"quest.Q039_RedEyedInvaders":                  nil,
		"quest.Q042_HelpTheUncle":                     nil,
		"quest.Q043_HelpTheSister":                    nil,
		"quest.Q044_HelpTheSon":                       nil,
		"quest.Q045_ToTalkingIsland":                  nil,
		"quest.Q046_OnceMoreInTheArmsOfTheMotherTree": nil,
		"quest.Q047_IntoTheDarkForest":                nil,
		"quest.Q048_ToTheImmortalPlateau":             nil,
		"quest.Q049_TheRoadHome":                      nil,
		"quest.Q050_LanoscosSpecialBait":              nil,
	}
}
