package catalog

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/warrior"
)

// aiIndividual2 is the catalog of batch 2 of 8 of the individual NPC behaviors (#187).
func aiIndividual2() script.Catalog {
	return script.Catalog{
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastDDMagic.Agit02DoomServantAgit":                                                           warrior.Agit02DoomServantAgit,
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastDDMagic.CorePrivateWarriorCastDDMagicAggressive.CorePrivatePerum":                        warrior.CorePrivatePerum,
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastDDMagic.CorePrivateWarriorCastDDMagicAggressive.CorePrivateWarriorCastDDMagicAggressive": warrior.CorePrivateCastDDMagicAggressive,
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastDDMagic.WarriorCastDDMagic":                                                              warrior.CastDDMagic,
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastDDMagic.WarriorCastDDMagicAggressive":                                                    warrior.CastDDMagicAggressive,
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastDDMagic.WarriorCastDDMagicSelfBuff":                                                      warrior.CastDDMagicSelfBuff,
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastDDMagic.WarriorCastDDMagicSelfBuffAggressive":                                            warrior.CastDDMagicSelfBuffAggressive,
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingCurse.Agit02DoomGuardAgit":                                                            warrior.Agit02DoomGuardAgit,
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingCurse.Agit02DoomKnightAgitParty":                                                      warrior.Agit02DoomKnightAgitParty,
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingCurse.Agit02VampireAgitParty":                                                         warrior.Agit02VampireAgitParty,
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingCurse.WarriorCastingCurse":                                                            warrior.CastingCurse,
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingCurse.WarriorCastingCurseAggressive.Agit02DoomKnightAgit":                             warrior.Agit02DoomKnightAgit,
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingCurse.WarriorCastingCurseAggressive.CorePrivateDicor":                                 warrior.CorePrivateDicor,
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingCurse.WarriorCastingCurseAggressive.SpecialBloodyQueen":                               nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingCurse.WarriorCastingCurseAggressive.WarriorCastingCurseAggressive":                    warrior.CastingCurseAggressive,
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingCurse.WarriorHereticSelfRangePhysical":                                                warrior.HereticSelfRangePhysical,
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingCurse.WarriorHereticSelfRangePhysicalAggressive":                                      warrior.HereticSelfRangePhysicalAggressive,
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingEnchant.Agit02DoomArcherAgit":                                                         nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingEnchant.Agit02DoomArcherAgitParty":                                                    nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingEnchant.Agit02DoomWarriorAgit":                                                        nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingEnchant.OrfenPrivateRaikel":                                                           nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingEnchant.WarriorCastingEnchant":                                                        nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingEnchant.WarriorCastingEnchantClan":                                                    nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingEnchant.WarriorCastingEnchantClanAggressive":                                          nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingEnchant.WarriorCastingEnchantSelfAggressive":                                          nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingEnchant.ZakenPrivateDollBlader":                                                       nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.RoyalRushWarriorPhysicalSpecial":                                             nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.RoyalRushWarriorStone":                                                       nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.SaintNinja":                                                                  nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.WarriorCorpseVampire":                                                        nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.WarriorCorpseVampireAggressive":                                              nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.WarriorCorpseVampireMobHate":                                                 nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.WarriorFleeFromClanAggressive":                                               nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.WarriorPassiveUsePowershot":                                                  nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.WarriorPatrolWeaponEquipped":                                                 nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.WarriorPhysicalSpecialAggressive":                                            nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.WarriorPhysicalSpecialAreaAggressive":                                        nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.WarriorPhysicalSpecialDebuffAggressive":                                      nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.WarriorPhysicalSpecialFollowing":                                             nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.WarriorPhysicalSpecialGatheringAggressive":                                   nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.WarriorPhysicalSpecialHoldAggressive":                                        nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.WarriorPhysicalSpecialPotionAggressive":                                      nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.WarriorPhysicalSpecialSpoilAggressive":                                       nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.WarriorPhysicalSpecialStoneAggressive":                                       nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.WarriorPhysicalSpecialStrongAggressive":                                      nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.WarriorPhysicalSpecialVelociraptor":                                          nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.WarriorSaintTransform":                                                       nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.WarriorUsePowershotAggressive":                                               nil,
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.ZakenPrivatePirateZombie":                                                    nil,
	}
}
