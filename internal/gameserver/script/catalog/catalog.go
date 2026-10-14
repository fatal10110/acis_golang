// Package catalog maps the content scripts to their constructors: the
// scripts.xml paths of the quests, features, teleporters and NPC behaviors,
// and the npcmaker types of the makers, that are ported after the engine.
//
// Each file is the catalog of one owner issue, so content lanes never edit
// the same file; a lane edits only its own file. A nil constructor marks an
// entry its owner has not ported yet: the composition root drops it, so a
// listed path stays unported (logged at error level and skipped at boot)
// and a maker type keeps the default maker. Porting an entry replaces its
// nil with the constructor.
package catalog

import (
	"maps"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/thirdclass"
)

// Scripts returns the script catalogs by catalog file. sagas is the family
// the saga core and every saga share.
func Scripts(sagas *thirdclass.Family) map[string]script.Catalog {
	return map[string]script.Catalog{
		"ai_boss_antharas":  aiBossAntharas(),
		"ai_boss_baium":     aiBossBaium(),
		"ai_boss_benom":     aiBossBenom(),
		"ai_boss_core":      aiBossCore(),
		"ai_boss_drchaos":   aiBossDrChaos(),
		"ai_boss_frintezza": aiBossFrintezza(),
		"ai_boss_orfen":     aiBossOrfen(),
		"ai_boss_queenant":  aiBossQueenAnt(),
		"ai_boss_sailren":   aiBossSailren(),
		"ai_boss_sirra":     aiBossSirra(),
		"ai_boss_valakas":   aiBossValakas(),
		"ai_boss_zaken":     aiBossZaken(),
		"ai_group":          aiGroup(),
		"ai_individual_1":   aiIndividual1(),
		"ai_individual_2":   aiIndividual2(),
		"ai_individual_3":   aiIndividual3(),
		"ai_individual_4":   aiIndividual4(),
		"ai_individual_5":   aiIndividual5(),
		"ai_individual_6":   aiIndividual6(),
		"ai_individual_7":   aiIndividual7(),
		"ai_individual_8":   aiIndividual8(),
		"ai_siegeguard":     aiSiegeGuard(),
		"features":          features(),
		"quests_001_050":    quests001To050(),
		"quests_051_100":    quests051To100(sagas),
		"quests_101_150":    quests101To150(),
		"quests_151_200":    quests151To200(),
		"quests_201_250":    quests201To250(),
		"quests_251_300":    quests251To300(),
		"quests_301_up":     quests301Up(),
		"siegablehall":      siegableHall(),
		"teleporters":       teleporters(),
	}
}

// Makers returns the maker catalogs by catalog file.
func Makers() map[string]script.MakerCatalog {
	return map[string]script.MakerCatalog{
		"makers_1": makers1(),
		"makers_2": makers2(),
		"makers_3": makers3(),
	}
}

// HandleOps returns, per script of the catalog files, the kind-specific NPC
// handle operations each of its hooks reaches, as generated beside each
// file.
func HandleOps() map[string]script.HandleOps {
	out := map[string]script.HandleOps{}
	for _, ops := range []map[string]script.HandleOps{
		aiBossAntharasHandleOps(),
		aiBossBaiumHandleOps(),
		aiBossBenomHandleOps(),
		aiBossCoreHandleOps(),
		aiBossDrChaosHandleOps(),
		aiBossFrintezzaHandleOps(),
		aiBossOrfenHandleOps(),
		aiBossQueenAntHandleOps(),
		aiBossSailrenHandleOps(),
		aiBossSirraHandleOps(),
		aiBossValakasHandleOps(),
		aiBossZakenHandleOps(),
		aiGroupHandleOps(),
		aiIndividual1HandleOps(),
		aiIndividual2HandleOps(),
		aiIndividual3HandleOps(),
		aiIndividual4HandleOps(),
		aiIndividual5HandleOps(),
		aiIndividual6HandleOps(),
		aiIndividual7HandleOps(),
		aiIndividual8HandleOps(),
		aiSiegeGuardHandleOps(),
		featuresHandleOps(),
		quests001To050HandleOps(),
		quests051To100HandleOps(),
		quests101To150HandleOps(),
		quests151To200HandleOps(),
		quests201To250HandleOps(),
		quests251To300HandleOps(),
		quests301UpHandleOps(),
		siegableHallHandleOps(),
		teleportersHandleOps(),
	} {
		maps.Copy(out, ops)
	}
	return out
}
