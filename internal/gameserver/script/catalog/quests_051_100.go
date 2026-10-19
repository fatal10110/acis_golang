package catalog

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q051"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q052"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q053"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/thirdclass"
)

// quests051To100 is the catalog of quests 051 to 100 with the sagas (#179), except the one the engine slices ported. sagas is the family the saga core and every saga share.
func quests051To100(sagas *thirdclass.Family) script.Catalog {
	return script.Catalog{
		"quest.Q051_OFullesSpecialBait":         q051.New,
		"quest.Q052_WilliesSpecialBait":         q052.New,
		"quest.Q053_LinnaeusSpecialBait":        q053.New,
		"quest.Q071_SagaOfEvasTemplar":          nil,
		"quest.Q072_SagaOfTheSwordMuse":         nil,
		"quest.Q073_SagaOfTheDuelist":           nil,
		"quest.Q074_SagaOfTheDreadnought":       nil,
		"quest.Q075_SagaOfTheTitan":             nil,
		"quest.Q076_SagaOfTheGrandKhavatari":    nil,
		"quest.Q077_SagaOfTheDominator":         nil,
		"quest.Q078_SagaOfTheDoomcryer":         nil,
		"quest.Q079_SagaOfTheAdventurer":        nil,
		"quest.Q080_SagaOfTheWindRider":         nil,
		"quest.Q081_SagaOfTheGhostHunter":       nil,
		"quest.Q082_SagaOfTheSagittarius":       nil,
		"quest.Q083_SagaOfTheMoonlightSentinel": nil,
		"quest.Q084_SagaOfTheGhostSentinel":     nil,
		"quest.Q085_SagaOfTheCardinal":          nil,
		"quest.Q086_SagaOfTheHierophant":        nil,
		"quest.Q087_SagaOfEvasSaint":            nil,
		"quest.Q088_SagaOfTheArchmage":          nil,
		"quest.Q089_SagaOfTheMysticMuse":        nil,
		"quest.Q090_SagaOfTheStormScreamer":     nil,
		"quest.Q091_SagaOfTheArcanaLord":        nil,
		"quest.Q092_SagaOfTheElementalMaster":   nil,
		"quest.Q093_SagaOfTheSpectralMaster":    nil,
		"quest.Q094_SagaOfTheSoultaker":         nil,
		"quest.Q095_SagaOfTheHellKnight":        nil,
		"quest.Q096_SagaOfTheSpectralDancer":    nil,
		"quest.Q097_SagaOfTheShillienTemplar":   nil,
		"quest.Q098_SagaOfTheShillienSaint":     nil,
		"quest.Q099_SagaOfTheFortuneSeeker":     nil,
		"quest.Q100_SagaOfTheMaestro":           nil,
	}
}
