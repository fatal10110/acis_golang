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
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivateCastEnchant":                                               warrior.PartyPrivateCastEnchant,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivateCastHeal":                                                  warrior.PartyPrivateCastHeal,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivateCastHealCurse":                                             warrior.PartyPrivateCastHealCurse,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivateCoupleFollower.PartyPrivateCoupleFollower":                 warrior.PartyPrivateCoupleFollower,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivateCoupleFollower.PartyPrivateCoupleFollowerBabyAggressive":   nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivateCoupleFollower.PartyPrivateCoupleFollowerFemaleAggressive": nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivateCouplePhysicalSpecial":                                     warrior.PartyPrivateCouplePhysicalSpecial,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivateCoward":                                                    warrior.PartyPrivateCoward,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivateHereticCastCurse":                                          warrior.PartyPrivateHereticCastCurse,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivatePabelgolem":                                                warrior.PartyPrivatePabelgolem,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivatePhysicalSpecial.PartyPrivateHereticZealot":                 nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivatePhysicalSpecial.PartyPrivateHereticZealot2":                nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivatePhysicalSpecial.PartyPrivatePhysicalSpecial":               nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivatePhysicalSpecial.PartyPrivatePhysicalSpecialHeal":           nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivatePhysicalSpecial.PartyPrivatePhysicalSpecialPowerShot":      nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivatePhysicalSpecial.PartyPrivatePrimeval":                      nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivatePhysicalSpecial.PartyPrivateSpawnPrivateAggressive":        nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivatePhysicalStone":                                             warrior.PartyPrivatePhysicalStone,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivatePriest":                                                    warrior.PartyPrivatePriest,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivatePriestBuffer":                                              warrior.PartyPrivatePriestBuffer,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivateSlowType2":                                                 warrior.PartyPrivateSlowType2,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivateSlowTypeMagic":                                             warrior.PartyPrivateSlowTypeMagic,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.PartyPrivateSplit":                                                     warrior.PartyPrivateSplit,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.QueenAntPrivateRoyalGuardAnt":                                          warrior.QueenAntPrivateRoyalGuardAnt,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.RoyalRushPartyPrivateBomb":                                             warrior.RoyalRushPartyPrivateBomb,
		"script.ai.individual.Monster.WarriorBase.Warrior.PartyPrivateWarrior.RoyalRushPartyPrivateHate":                                             warrior.RoyalRushPartyPrivateHate,
	}
}
