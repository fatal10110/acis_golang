package catalog

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q257"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q258"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q259"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q261"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q262"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q263"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q264"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q265"
)

// quests251To300 is the catalog of quests 251 to 300 (#183), except the one the engine slices ported.
func quests251To300() script.Catalog {
	return script.Catalog{
		"quest.Q257_TheGuardIsBusy":            q257.New,
		"quest.Q258_BringWolfPelts":            q258.New,
		"quest.Q259_RanchersPlea":              q259.New,
		"quest.Q261_CollectorsDream":           q261.New,
		"quest.Q262_TradeWithTheIvoryTower":    q262.New,
		"quest.Q263_OrcSubjugation":            q263.New,
		"quest.Q264_KeenClaws":                 q264.New,
		"quest.Q265_ChainsOfSlavery":           q265.New,
		"quest.Q266_PleasOfPixies":             nil,
		"quest.Q267_WrathOfVerdure":            nil,
		"quest.Q271_ProofOfValor":              nil,
		"quest.Q272_WrathOfAncestors":          nil,
		"quest.Q273_InvadersOfTheHolyLand":     nil,
		"quest.Q274_SkirmishWithTheWerewolves": nil,
		"quest.Q275_DarkWingedSpies":           nil,
		"quest.Q276_TotemOfTheHestui":          nil,
		"quest.Q277_GatekeepersOffering":       nil,
		"quest.Q291_RevengeOfTheRedbonnet":     nil,
		"quest.Q292_BrigandsSweep":             nil,
		"quest.Q293_TheHiddenVeins":            nil,
		"quest.Q294_CovertBusiness":            nil,
		"quest.Q295_DreamingOfTheSkies":        nil,
		"quest.Q296_TarantulasSpiderSilk":      nil,
		"quest.Q297_GatekeepersFavor":          nil,
		"quest.Q298_LizardmensConspiracy":      nil,
		"quest.Q299_GatherIngredientsForPie":   nil,
		"quest.Q300_HuntingLetoLizardman":      nil,
	}
}
