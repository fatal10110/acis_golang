package catalog

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q101"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q102"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q103"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q104"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q105"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q106"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q107"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q108"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q109"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q110"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q111"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q112"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q113"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q114"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q116"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q117"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q119"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q120"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q121"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q122"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q124"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q125"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q126"
)

// quests101To150 is the catalog of quests 101 to 150 (#180).
func quests101To150() script.Catalog {
	return script.Catalog{
		"quest.Q101_SwordOfSolidarity":          q101.New,
		"quest.Q102_SeaOfSporesFever":           q102.New,
		"quest.Q103_SpiritOfCraftsman":          q103.New,
		"quest.Q104_SpiritOfMirrors":            q104.New,
		"quest.Q105_SkirmishWithTheOrcs":        q105.New,
		"quest.Q106_ForgottenTruth":             q106.New,
		"quest.Q107_MercilessPunishment":        q107.New,
		"quest.Q108_JumbleTumbleDiamondFuss":    q108.New,
		"quest.Q109_InSearchOfTheNest":          q109.New,
		"quest.Q110_ToThePrimevalIsle":          q110.New,
		"quest.Q111_ElrokianHuntersProof":       q111.New,
		"quest.Q112_WalkOfFate":                 q112.New,
		"quest.Q113_StatusOfTheBeaconTower":     q113.New,
		"quest.Q114_ResurrectionOfAnOldManager": q114.New,
		"quest.Q115_TheOtherSideOfTruth":        nil,
		"quest.Q116_BeyondTheHillsOfWinter":     q116.New,
		"quest.Q117_TheOceanOfDistantStars":     q117.New,
		"quest.Q118_ToLeadAndBeLed":             nil,
		"quest.Q119_LastImperialPrince":         q119.New,
		"quest.Q120_PavelsResearch":             q120.New,
		"quest.Q121_PavelTheGiant":              q121.New,
		"quest.Q122_OminousNews":                q122.New,
		"quest.Q123_TheLeaderAndTheFollower":    nil,
		"quest.Q124_MeetingTheElroki":           q124.New,
		"quest.Q125_TheNameOfEvil_1":            q125.New,
		"quest.Q126_TheNameOfEvil_2":            q126.New,
		"quest.Q127_KamaelAWindowToTheFuture":   nil,
	}
}
