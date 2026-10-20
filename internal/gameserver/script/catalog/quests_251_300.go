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
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q266"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q267"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q271"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q272"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q273"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q274"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q277"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q291"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q292"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q293"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q294"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q295"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q296"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q297"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q298"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q299"
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
		"quest.Q266_PleasOfPixies":             q266.New,
		"quest.Q267_WrathOfVerdure":            q267.New,
		"quest.Q271_ProofOfValor":              q271.New,
		"quest.Q272_WrathOfAncestors":          q272.New,
		"quest.Q273_InvadersOfTheHolyLand":     q273.New,
		"quest.Q274_SkirmishWithTheWerewolves": q274.New,
		"quest.Q275_DarkWingedSpies":           nil,
		"quest.Q276_TotemOfTheHestui":          nil,
		"quest.Q277_GatekeepersOffering":       q277.New,
		"quest.Q291_RevengeOfTheRedbonnet":     q291.New,
		"quest.Q292_BrigandsSweep":             q292.New,
		"quest.Q293_TheHiddenVeins":            q293.New,
		"quest.Q294_CovertBusiness":            q294.New,
		"quest.Q295_DreamingOfTheSkies":        q295.New,
		"quest.Q296_TarantulasSpiderSilk":      q296.New,
		"quest.Q297_GatekeepersFavor":          q297.New,
		"quest.Q298_LizardmensConspiracy":      q298.New,
		"quest.Q299_GatherIngredientsForPie":   q299.New,
		"quest.Q300_HuntingLetoLizardman":      nil,
	}
}
