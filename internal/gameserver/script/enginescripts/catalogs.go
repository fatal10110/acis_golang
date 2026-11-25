// Package enginescripts holds the catalogs of the scripts ported with the engine,
// as the server boot and the load harness join them with the content catalogs.
package enginescripts

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/group"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/guard"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/monster"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/monsterbehavior"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/raid"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/warrior"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/wizard"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/feature/alliance"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/feature/blackjudge"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/feature/cabalbuffer"
	featureclan "github.com/fatal10110/acis_golang/internal/gameserver/script/feature/clan"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/feature/newbiehelper"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/feature/tutorial"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/clanrep"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/ketravarka"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q001"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q023"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q070"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q211"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q230"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q234"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q235"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q260"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q316"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q350"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q360"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q422"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q636"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q646"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q651"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/thirdclass"
	scripttask "github.com/fatal10110/acis_golang/internal/gameserver/script/task"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/teleport/noblesseteleporter"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/teleport/primevalsummoner"
)

// Catalogs are the catalogs of the scripts ported with the engine: quests,
// scripted features, teleporters, scheduled tasks and the base NPC behaviors.
// crystals are the soul crystals a quest charges, buffs the buffs the newbie
// guides cast; the saga core and the sagas share sagas.
func Catalogs(crystals *item.SoulCrystalTable, buffs *skill.NewbieBuffTable, sagas *thirdclass.Family) []script.Catalog {
	return []script.Catalog{Quests(crystals, sagas), Features(buffs), Teleporters(), Tasks(), Behaviors()}
}

// Quests lists the quests ported with the engine.
func Quests(crystals *item.SoulCrystalTable, sagas *thirdclass.Family) script.Catalog {
	return script.Catalog{
		"quest.Q001_LettersOfLove":          q001.New,
		"quest.Q023_LidiasHeart":            q023.New,
		"quest.Q070_SagaOfThePhoenixKnight": func() script.Script { return q070.New(sagas) },
		"quest.Q211_TrialOfTheChallenger":   q211.New,
		"quest.Q230_TestOfTheSummoner":      q230.New,
		"quest.Q234_FatesWhisper":           q234.New,
		"quest.Q235_MimirsElixir":           q235.New,
		"quest.Q260_HuntTheOrcs":            q260.New,
		"quest.Q316_DestroyPlagueCarriers":  q316.New,
		"quest.Q350_EnhanceYourWeapon":      func() script.Script { return q350.New(crystals) },
		"quest.Q360_PlunderTheirSupplies":   q360.New,
		"quest.Q422_RepentYourSins":         q422.New,
		"quest.Q508_AClansReputation":       clanrep.Q508,
		"quest.Q510_AClansReputation":       clanrep.Q510,
		"quest.Q607_ProveYourCourage":       ketravarka.Q607,
		"quest.Q636_TruthBeyondTheGate":     q636.New,
		"quest.Q646_SignsOfRevolt":          q646.New,
		"quest.Q651_RunawayYouth":           q651.New,
		"quest.ThirdClassQuest":             func() script.Script { return thirdclass.NewCore(sagas) },
	}
}

// Features lists the scripted features; the newbie guides cast
// buffs.
func Features(buffs *skill.NewbieBuffTable) script.Catalog {
	return script.Catalog{
		"script.feature.Alliance":     alliance.New,
		"script.feature.BlackJudge":   blackjudge.New,
		"script.feature.CabalBuffer":  cabalbuffer.New,
		"script.feature.Clan":         featureclan.New,
		"script.feature.NewbieHelper": func() script.Script { return newbiehelper.New(buffs) },
		"script.feature.Tutorial":     tutorial.New,
	}
}

// Teleporters lists the scripted teleporters.
func Teleporters() script.Catalog {
	return script.Catalog{
		"script.teleport.NoblesseTeleporter": noblesseteleporter.New,
		"script.teleport.PrimevalSummoner":   primevalsummoner.New,
	}
}

// Behaviors lists the NPC behaviors.
func Behaviors() script.Catalog {
	return script.Catalog{
		"script.ai.group.Walkers":                                                                        group.Walkers,
		"script.ai.individual.DefaultNpc":                                                                defaultnpc.New,
		"script.ai.individual.Guard.Guard":                                                               guard.Guard,
		"script.ai.individual.Guard.GuardMoveAround":                                                     guard.MoveAround,
		"script.ai.individual.Guard.GuardStand":                                                          guard.Stand,
		"script.ai.individual.Monster.MonsterAI":                                                         monster.New,
		"script.ai.individual.Monster.MonsterBehavior.MonsterBehavior":                                   monsterbehavior.MonsterBehavior,
		"script.ai.individual.Monster.MonsterBehavior.WarriorBehavior.WarriorBehavior":                   monsterbehavior.WarriorBehavior,
		"script.ai.individual.Monster.MonsterBehavior.WizardBehavior.WizardBehavior":                     monsterbehavior.WizardBehavior,
		"script.ai.individual.Monster.RaidPrivate.RaidPrivateStandard":                                   raid.PrivateStandard,
		"script.ai.individual.Monster.WarriorBase.WarriorBase":                                           warrior.Base,
		"script.ai.individual.Monster.WarriorBase.Warrior.Warrior":                                       warrior.Warrior,
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorAggressive.WarriorAggressive":           warrior.Aggressive,
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorHold.WarriorHold":                       warrior.Hold,
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.WarriorPhysicalSpecial": warrior.PhysicalSpecial,
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.WarriorFishingBlock":    warrior.FishingBlock,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderWarrior":         warrior.PartyLeader,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivateWarrior":       warrior.PartyPrivate,
		"script.ai.individual.Monster.WizardBase.WizardBase":                                             wizard.Base,
		"script.ai.individual.Monster.WizardBase.Wizard.Wizard":                                          wizard.Wizard,
	}
}

// Tasks lists the scheduled tasks. The shutdown and restart tasks
// run only once an operator uncomments their scripts.xml entries.
func Tasks() script.Catalog {
	return script.Catalog{
		"task.CastleTaxRefresh":     scripttask.CastleTaxRefresh,
		"task.ClanLadderRefresh":    scripttask.ClanLadderRefresh,
		"task.ClanLeaderTransfer":   scripttask.ClanLeaderTransfer,
		"task.RaidPointReset":       scripttask.RaidPointReset,
		"task.RecommendationUpdate": scripttask.RecommendationUpdate,
		"task.ServerRestart":        scripttask.ServerRestart,
		"task.ServerShutdown":       scripttask.ServerShutdown,
		"task.SevenSignsUpdate":     scripttask.SevenSignsUpdate,
	}
}
