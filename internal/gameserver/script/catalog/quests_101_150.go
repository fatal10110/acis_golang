package catalog

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q101"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q102"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q103"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q104"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q105"
)

// quests101To150 is the catalog of quests 101 to 150 (#180).
func quests101To150() script.Catalog {
	return script.Catalog{
		"quest.Q101_SwordOfSolidarity":          q101.New,
		"quest.Q102_SeaOfSporesFever":           q102.New,
		"quest.Q103_SpiritOfCraftsman":          q103.New,
		"quest.Q104_SpiritOfMirrors":            q104.New,
		"quest.Q105_SkirmishWithTheOrcs":        q105.New,
		"quest.Q106_ForgottenTruth":             nil,
		"quest.Q107_MercilessPunishment":        nil,
		"quest.Q108_JumbleTumbleDiamondFuss":    nil,
		"quest.Q109_InSearchOfTheNest":          nil,
		"quest.Q110_ToThePrimevalIsle":          nil,
		"quest.Q111_ElrokianHuntersProof":       nil,
		"quest.Q112_WalkOfFate":                 nil,
		"quest.Q113_StatusOfTheBeaconTower":     nil,
		"quest.Q114_ResurrectionOfAnOldManager": nil,
		"quest.Q115_TheOtherSideOfTruth":        nil,
		"quest.Q116_BeyondTheHillsOfWinter":     nil,
		"quest.Q117_TheOceanOfDistantStars":     nil,
		"quest.Q118_ToLeadAndBeLed":             nil,
		"quest.Q119_LastImperialPrince":         nil,
		"quest.Q120_PavelsResearch":             nil,
		"quest.Q121_PavelTheGiant":              nil,
		"quest.Q122_OminousNews":                nil,
		"quest.Q123_TheLeaderAndTheFollower":    nil,
		"quest.Q124_MeetingTheElroki":           nil,
		"quest.Q125_TheNameOfEvil_1":            nil,
		"quest.Q126_TheNameOfEvil_2":            nil,
		"quest.Q127_KamaelAWindowToTheFuture":   nil,
	}
}
