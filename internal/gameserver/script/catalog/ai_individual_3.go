package catalog

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/warrior"
)

// aiIndividual3 is the catalog of batch 3 of 8 of the individual NPC behaviors (#188).
func aiIndividual3() script.Catalog {
	return script.Catalog{
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.FreyaGardener":                                                                         warrior.FreyaGardener,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.FreyaPet":                                                                              warrior.FreyaPet,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.FreyaSecretaryLeader":                                                                  warrior.FreyaSecretaryLeader,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderBombSpawn":                                                                  warrior.PartyLeaderBombSpawn,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderCannibaliAggressiva":                                                        warrior.PartyLeaderCannibaliAggressiva,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderCastCurseAggressive":                                                        warrior.PartyLeaderCastCurseAggressive,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderCastDD":                                                                     warrior.PartyLeaderCastDD,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderCastDDAggressive":                                                           warrior.PartyLeaderCastDDAggressive,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderCastDDMagicStoneAggressive":                                                 warrior.PartyLeaderCastDDMagicStoneAggressive,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderCastSplashAggressive":                                                       warrior.PartyLeaderCastSplashAggressive,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderCasting3SkillsMagicalAggressive.AntharasPrivateBehemothDragon":              warrior.AntharasPrivateBehemothDragon,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderCasting3SkillsMagicalAggressive.PartyLeaderCasting3SkillsMagicalAggressive": warrior.PartyLeaderCasting3SkillsMagicalAggressive,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderCoupleCaptain.PartyLeaderCoupleCaptain":                                     nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderCoupleCaptain.PartyLeaderCoupleCaptainMaleAggressive":                       nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderCoupleCaptain.PartyLeaderCoupleCaptainNurseAggressive":                      nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderCouplePhysicalSpecial":                                                      warrior.PartyLeaderCouplePhysicalSpecial,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderPabelgolemAggressive":                                                       warrior.PartyLeaderPabelgolemAggressive,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderPatrol":                                                                     warrior.PartyLeaderPatrol,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderPatrolPhysicalSpecialStoneAggressive":                                       warrior.PartyLeaderPatrolPhysicalSpecialStoneAggressive,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderPhysicalSpecial":                                                            warrior.PartyLeaderPhysicalSpecial,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderPhysicalSpecial2.PartyLeaderHeretic2Aggressive":                             nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderPhysicalSpecial2.PartyLeaderHereticAggressive":                              nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderPhysicalSpecial2.PartyLeaderPhysicalSpecial2":                               nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderPhysicalSpecialAggressive":                                                  warrior.PartyLeaderPhysicalSpecialAggressive,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderPhysicalSpecialSummon":                                                      warrior.PartyLeaderPhysicalSpecialSummon,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderPhysicalSpecialSummonAggressive":                                            warrior.PartyLeaderPhysicalSpecialSummonAggressive,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderSlowTypeAggressive":                                                         warrior.PartyLeaderSlowTypeAggressive,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderWarriorAggressive":                                                          warrior.PartyLeaderWarriorAggressive,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PrimevalRoamer.PrimevalRoamer":                                                         nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PrimevalRoamer.PrimevalRoamerWalker":                                                   nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.QuestPartyLeader":                                                                      warrior.QuestPartyLeader,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.RoyalRushPartyLeader":                                                                  warrior.RoyalRushPartyLeader,
	}
}
