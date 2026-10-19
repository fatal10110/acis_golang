package catalog

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q051"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q052"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q053"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q071"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q072"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q073"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q074"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q075"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q076"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q077"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q078"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q079"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q080"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q081"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q082"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q083"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q084"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q085"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q086"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q087"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q088"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q089"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q090"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q091"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q092"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q093"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q094"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q095"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q096"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q097"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q098"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q099"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q100"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/thirdclass"
)

// quests051To100 is the catalog of quests 051 to 100 with the sagas (#179), except the one the engine slices ported. sagas is the family the saga core and every saga share.
func quests051To100(sagas *thirdclass.Family) script.Catalog {
	return script.Catalog{
		"quest.Q051_OFullesSpecialBait":         q051.New,
		"quest.Q052_WilliesSpecialBait":         q052.New,
		"quest.Q053_LinnaeusSpecialBait":        q053.New,
		"quest.Q071_SagaOfEvasTemplar":          func() script.Script { return q071.New(sagas) },
		"quest.Q072_SagaOfTheSwordMuse":         func() script.Script { return q072.New(sagas) },
		"quest.Q073_SagaOfTheDuelist":           func() script.Script { return q073.New(sagas) },
		"quest.Q074_SagaOfTheDreadnought":       func() script.Script { return q074.New(sagas) },
		"quest.Q075_SagaOfTheTitan":             func() script.Script { return q075.New(sagas) },
		"quest.Q076_SagaOfTheGrandKhavatari":    func() script.Script { return q076.New(sagas) },
		"quest.Q077_SagaOfTheDominator":         func() script.Script { return q077.New(sagas) },
		"quest.Q078_SagaOfTheDoomcryer":         func() script.Script { return q078.New(sagas) },
		"quest.Q079_SagaOfTheAdventurer":        func() script.Script { return q079.New(sagas) },
		"quest.Q080_SagaOfTheWindRider":         func() script.Script { return q080.New(sagas) },
		"quest.Q081_SagaOfTheGhostHunter":       func() script.Script { return q081.New(sagas) },
		"quest.Q082_SagaOfTheSagittarius":       func() script.Script { return q082.New(sagas) },
		"quest.Q083_SagaOfTheMoonlightSentinel": func() script.Script { return q083.New(sagas) },
		"quest.Q084_SagaOfTheGhostSentinel":     func() script.Script { return q084.New(sagas) },
		"quest.Q085_SagaOfTheCardinal":          func() script.Script { return q085.New(sagas) },
		"quest.Q086_SagaOfTheHierophant":        func() script.Script { return q086.New(sagas) },
		"quest.Q087_SagaOfEvasSaint":            func() script.Script { return q087.New(sagas) },
		"quest.Q088_SagaOfTheArchmage":          func() script.Script { return q088.New(sagas) },
		"quest.Q089_SagaOfTheMysticMuse":        func() script.Script { return q089.New(sagas) },
		"quest.Q090_SagaOfTheStormScreamer":     func() script.Script { return q090.New(sagas) },
		"quest.Q091_SagaOfTheArcanaLord":        func() script.Script { return q091.New(sagas) },
		"quest.Q092_SagaOfTheElementalMaster":   func() script.Script { return q092.New(sagas) },
		"quest.Q093_SagaOfTheSpectralMaster":    func() script.Script { return q093.New(sagas) },
		"quest.Q094_SagaOfTheSoultaker":         func() script.Script { return q094.New(sagas) },
		"quest.Q095_SagaOfTheHellKnight":        func() script.Script { return q095.New(sagas) },
		"quest.Q096_SagaOfTheSpectralDancer":    func() script.Script { return q096.New(sagas) },
		"quest.Q097_SagaOfTheShillienTemplar":   func() script.Script { return q097.New(sagas) },
		"quest.Q098_SagaOfTheShillienSaint":     func() script.Script { return q098.New(sagas) },
		"quest.Q099_SagaOfTheFortuneSeeker":     func() script.Script { return q099.New(sagas) },
		"quest.Q100_SagaOfTheMaestro":           func() script.Script { return q100.New(sagas) },
	}
}
