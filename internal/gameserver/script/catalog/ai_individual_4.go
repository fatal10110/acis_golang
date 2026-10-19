package catalog

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/warrior"
)

// aiIndividual4 is the catalog of batch 4 of 8 of the individual NPC behaviors (#189).
func aiIndividual4() script.Catalog {
	return script.Catalog{
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.FreyaBear":                                                             warrior.FreyaBear,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.OrfenPrivateRaikelLeos":                                                warrior.OrfenPrivateRaikelLeos,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivateCannibaliAggressive":                                       warrior.PartyPrivateCannibaliAggressive,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivateCast3SkillsMagical":                                        warrior.PartyPrivateCast3SkillsMagical,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivateCastCurse":                                                 warrior.PartyPrivateCastCurse,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivateCastDDHeal":                                                warrior.PartyPrivateCastDDHeal,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivateCastDDMagicCurseStone":                                     warrior.PartyPrivateCastDDMagicCurseStone,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivateCastEnchant":                                               nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivateCastHeal":                                                  nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivateCastHealCurse":                                             nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivateCoupleFollower.PartyPrivateCoupleFollower":                 nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivateCoupleFollower.PartyPrivateCoupleFollowerBabyAggressive":   nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivateCoupleFollower.PartyPrivateCoupleFollowerFemaleAggressive": nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivateCouplePhysicalSpecial":                                     nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivateCoward":                                                    nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivateHereticCastCurse":                                          warrior.PartyPrivateHereticCastCurse,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivatePabelgolem":                                                nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivatePhysicalSpecial.PartyPrivateHereticZealot":                 nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivatePhysicalSpecial.PartyPrivateHereticZealot2":                nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivatePhysicalSpecial.PartyPrivatePhysicalSpecial":               nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivatePhysicalSpecial.PartyPrivatePhysicalSpecialHeal":           nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivatePhysicalSpecial.PartyPrivatePhysicalSpecialPowerShot":      nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivatePhysicalSpecial.PartyPrivatePrimeval":                      nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivatePhysicalSpecial.PartyPrivateSpawnPrivateAggressive":        nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivatePhysicalStone":                                             nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivatePriest":                                                    nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivatePriestBuffer":                                              nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivateSlowType2":                                                 nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivateSlowTypeMagic":                                             nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivateSplit":                                                     nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.QueenAntPrivateRoyalGuardAnt":                                          nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.RoyalRushPartyPrivateBomb":                                             nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.RoyalRushPartyPrivateHate":                                             nil,
	}
}
