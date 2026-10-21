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
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q018"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q019"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q020"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q027"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q028"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q029"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q030"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q031"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q032"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q033"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q034"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q035"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q036"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q037"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q038"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q039"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q042"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q043"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q044"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q045"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q046"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q047"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q048"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q049"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q050"
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
		"quest.Q018_MeetingWithTheGoldenRam":          q018.New,
		"quest.Q019_GoToThePastureland":               q019.New,
		"quest.Q020_BringUpWithLove":                  q020.New,
		"quest.Q021_HiddenTruth":                      nil,
		"quest.Q022_TragedyInVonHellmannForest":       nil,
		"quest.Q024_InhabitantsOfTheForestOfTheDead":  nil,
		"quest.Q025_HidingBehindTheTruth":             nil,
		"quest.Q027_ChestCaughtWithABaitOfWind":       q027.New,
		"quest.Q028_ChestCaughtWithABaitOfIcyAir":     q028.New,
		"quest.Q029_ChestCaughtWithABaitOfEarth":      q029.New,
		"quest.Q030_ChestCaughtWithABaitOfFire":       q030.New,
		"quest.Q031_SecretBuriedInTheSwamp":           q031.New,
		"quest.Q032_AnObviousLie":                     q032.New,
		"quest.Q033_MakeAPairOfDressShoes":            q033.New,
		"quest.Q034_InSearchOfCloth":                  q034.New,
		"quest.Q035_FindGlitteringJewelry":            q035.New,
		"quest.Q036_MakeASewingKit":                   q036.New,
		"quest.Q037_MakeFormalWear":                   q037.New,
		"quest.Q038_DragonFangs":                      q038.New,
		"quest.Q039_RedEyedInvaders":                  q039.New,
		"quest.Q042_HelpTheUncle":                     q042.New,
		"quest.Q043_HelpTheSister":                    q043.New,
		"quest.Q044_HelpTheSon":                       q044.New,
		"quest.Q045_ToTalkingIsland":                  q045.New,
		"quest.Q046_OnceMoreInTheArmsOfTheMotherTree": q046.New,
		"quest.Q047_IntoTheDarkForest":                q047.New,
		"quest.Q048_ToTheImmortalPlateau":             q048.New,
		"quest.Q049_TheRoadHome":                      q049.New,
		"quest.Q050_LanoscosSpecialBait":              q050.New,
	}
}
