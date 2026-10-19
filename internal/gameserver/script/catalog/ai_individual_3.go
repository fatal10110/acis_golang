package catalog

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/warrior"
)

// aiIndividual3 is the catalog of batch 3 of 8 of the individual NPC behaviors (#188).
func aiIndividual3() script.Catalog {
	return script.Catalog{
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.FreyaGardener":                                                                         nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.FreyaPet":                                                                              nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.FreyaSecretaryLeader":                                                                  nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderBombSpawn":                                                                  warrior.PartyLeaderBombSpawn,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderCannibaliAggressiva":                                                        nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderCastCurseAggressive":                                                        nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderCastDD":                                                                     warrior.PartyLeaderCastDD,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderCastDDAggressive":                                                           warrior.PartyLeaderCastDDAggressive,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderCastDDMagicStoneAggressive":                                                 warrior.PartyLeaderCastDDMagicStoneAggressive,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderCastSplashAggressive":                                                       nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderCasting3SkillsMagicalAggressive.AntharasPrivateBehemothDragon":              nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderCasting3SkillsMagicalAggressive.PartyLeaderCasting3SkillsMagicalAggressive": nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderCoupleCaptain.PartyLeaderCoupleCaptain":                                     nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderCoupleCaptain.PartyLeaderCoupleCaptainMaleAggressive":                       nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderCoupleCaptain.PartyLeaderCoupleCaptainNurseAggressive":                      nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderCouplePhysicalSpecial":                                                      warrior.PartyLeaderCouplePhysicalSpecial,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderPabelgolemAggressive":                                                       nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderPatrol":                                                                     warrior.PartyLeaderPatrol,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderPatrolPhysicalSpecialStoneAggressive":                                       nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderPhysicalSpecial":                                                            warrior.PartyLeaderPhysicalSpecial,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderPhysicalSpecial2.PartyLeaderHeretic2Aggressive":                             nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderPhysicalSpecial2.PartyLeaderHereticAggressive":                              nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderPhysicalSpecial2.PartyLeaderPhysicalSpecial2":                               nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderPhysicalSpecialAggressive":                                                  warrior.PartyLeaderPhysicalSpecialAggressive,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderPhysicalSpecialSummon":                                                      nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderPhysicalSpecialSummonAggressive":                                            nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderSlowTypeAggressive":                                                         nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderWarriorAggressive":                                                          nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PrimevalRoamer.PrimevalRoamer":                                                         nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PrimevalRoamer.PrimevalRoamerWalker":                                                   nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.QuestPartyLeader":                                                                      nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.RoyalRushPartyLeader":                                                                  nil,
	}
}
