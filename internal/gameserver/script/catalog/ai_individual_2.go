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
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingCurse.WarriorCastingCurseAggressive.SpecialBloodyQueen":                               warrior.SpecialBloodyQueen,
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingCurse.WarriorCastingCurseAggressive.WarriorCastingCurseAggressive":                    warrior.CastingCurseAggressive,
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingCurse.WarriorHereticSelfRangePhysical":                                                warrior.HereticSelfRangePhysical,
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingCurse.WarriorHereticSelfRangePhysicalAggressive":                                      warrior.HereticSelfRangePhysicalAggressive,
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingEnchant.Agit02DoomArcherAgit":                                                         warrior.Agit02DoomArcherAgit,
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingEnchant.Agit02DoomArcherAgitParty":                                                    warrior.Agit02DoomArcherAgitParty,
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingEnchant.Agit02DoomWarriorAgit":                                                        warrior.Agit02DoomWarriorAgit,
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingEnchant.OrfenPrivateRaikel":                                                           warrior.OrfenPrivateRaikel,
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingEnchant.WarriorCastingEnchant":                                                        warrior.CastingEnchant,
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingEnchant.WarriorCastingEnchantClan":                                                    warrior.CastingEnchantClan,
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingEnchant.WarriorCastingEnchantClanAggressive":                                          warrior.CastingEnchantClanAggressive,
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingEnchant.WarriorCastingEnchantSelfAggressive":                                          warrior.CastingEnchantSelfAggressive,
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingEnchant.ZakenPrivateDollBlader":                                                       warrior.ZakenPrivateDollBlader,
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.RoyalRushWarriorPhysicalSpecial":                                             warrior.RoyalRushWarriorPhysicalSpecial,
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.RoyalRushWarriorStone":                                                       warrior.RoyalRushWarriorStone,
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.SaintNinja":                                                                  warrior.SaintNinja,
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.WarriorCorpseVampire":                                                        warrior.CorpseVampire,
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.WarriorCorpseVampireAggressive":                                              warrior.CorpseVampireAggressive,
		"script.ai.individual.Monster.WarriorBase.Warrior.WarriorPhysicalSpecial.WarriorCorpseVampireMobHate":                                                 warrior.CorpseVampireMobHate,
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
