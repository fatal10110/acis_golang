// Package catalog maps every script to its constructor: the scripts.xml
// paths of the quests, features, teleporters, scheduled tasks and NPC
// behaviors, and the npcmaker types of the makers. engine.go holds those
// ported with the engine, the other files those ported after it.
//
// Each file is the catalog of one owner issue, so content lanes never edit
// the same file; a lane edits only its own file.
package catalog

import (
	"fmt"
	"maps"
	"slices"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/thirdclass"
)

// ScriptCatalog joins the script catalog files into the one catalog the
// server boots with; a path in two files is an error. crystals are the soul
// crystals a quest charges, buffs the buffs the newbie guides cast.
func ScriptCatalog(crystals *item.SoulCrystalTable, buffs *skill.NewbieBuffTable) (script.Catalog, error) {
	return join("script", Scripts(crystals, buffs, thirdclass.NewFamily()))
}

// MakerCatalog joins the maker catalog files into one, as ScriptCatalog
// joins the script catalog files.
func MakerCatalog() (script.MakerCatalog, error) {
	return join("maker type", Makers())
}

// join joins files into one; a key in two files is an error.
func join[C ~map[string]func() T, T any](what string, files map[string]C) (C, error) {
	out := C{}
	owner := map[string]string{}
	for _, name := range slices.Sorted(maps.Keys(files)) {
		for key, ctor := range files[name] {
			if first, dup := owner[key]; dup {
				return nil, fmt.Errorf("%s %s is in catalogs %s and %s", what, key, first, name)
			}
			owner[key] = name
			out[key] = ctor
		}
	}
	return out, nil
}

// Scripts returns the script catalogs by catalog file. crystals and buffs
// are as ScriptCatalog takes them; sagas is the family the saga core and
// every saga share.
func Scripts(crystals *item.SoulCrystalTable, buffs *skill.NewbieBuffTable, sagas *thirdclass.Family) map[string]script.Catalog {
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
		"engine":            engine(crystals, buffs, sagas),
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
		"engine":   engineMakers(),
		"makers_1": makers1(),
		"makers_2": makers2(),
		"makers_3": makers3(),
	}
}

// HandleOps returns, per script of the catalog files, the kind-specific NPC
// handle operations each of its hooks reaches, as generated beside each
// file; the seam gate reads them at boot.
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
		engineHandleOps(),
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
