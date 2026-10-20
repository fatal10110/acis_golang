package catalog

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/monster"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/monsterbehavior"
)

// aiIndividual7 is the catalog of batch 7 of 8 of the individual NPC behaviors (#192).
func aiIndividual7() script.Catalog {
	return script.Catalog{
		"script.ai.individual.Monster.LV3Monster.LV3Dagger":                                                                 monster.LV3Dagger,
		"script.ai.individual.Monster.LV3Monster.LV3Dwarf":                                                                  monster.LV3Dwarf,
		"script.ai.individual.Monster.LV3Monster.LV3Healer":                                                                 monster.LV3Healer,
		"script.ai.individual.Monster.LV3Monster.LV3Knight":                                                                 monster.LV3Knight,
		"script.ai.individual.Monster.LV3Monster.LV3Monster":                                                                monster.LV3Monster,
		"script.ai.individual.Monster.LV3Monster.LV3Orc":                                                                    monster.LV3Orc,
		"script.ai.individual.Monster.LV3Monster.LV3PartyLeaderMonster":                                                     monster.LV3PartyLeader,
		"script.ai.individual.Monster.LV3Monster.LV3PartyPrivateMonster":                                                    monster.LV3PartyPrivate,
		"script.ai.individual.Monster.LV3Monster.LV3Ranger":                                                                 monster.LV3Ranger,
		"script.ai.individual.Monster.LV3Monster.LV3SongDance":                                                              monster.LV3SongDance,
		"script.ai.individual.Monster.LV3Monster.LV3Summoner":                                                               monster.LV3Summoner,
		"script.ai.individual.Monster.LV3Monster.LV3Wizard":                                                                 monster.LV3Wizard,
		"script.ai.individual.Monster.LV3Monster.LV3WizardPrivate":                                                          monster.LV3WizardPrivate,
		"script.ai.individual.Monster.MonsterBehavior.WarriorBehavior.WarriorUseSkill.WarriorUseSkill":                      nil,
		"script.ai.individual.Monster.MonsterBehavior.WizardBehavior.WizardUseSkill.WizardUseSkill":                         monsterbehavior.WizardUseSkill,
		"script.ai.individual.Monster.RaidBoss.RaidBossAlone.RaidBossType1.Gordon":                                          nil,
		"script.ai.individual.Monster.RaidBoss.RaidBossAlone.RaidBossType1.RaidBossAloneSummonPrivate":                      nil,
		"script.ai.individual.Monster.RaidBoss.RaidBossAlone.RaidBossType1.RaidBossForTeleportDungeon":                      nil,
		"script.ai.individual.Monster.RaidBoss.RaidBossAlone.RaidBossType1.RaidBossType1":                                   nil,
		"script.ai.individual.Monster.RaidBoss.RaidBossAlone.RaidBossType1.RaidBossType1Aggressive.DrChaosGiganticGolem":    nil,
		"script.ai.individual.Monster.RaidBoss.RaidBossAlone.RaidBossType1.RaidBossType1Aggressive.FreyaRoyalGuard":         nil,
		"script.ai.individual.Monster.RaidBoss.RaidBossAlone.RaidBossType1.RaidBossType1Aggressive.RaidBossType1Aggressive": nil,
		"script.ai.individual.Monster.RaidBoss.RaidBossAlone.RaidBossType1.Sailren":                                         nil,
		"script.ai.individual.Monster.RaidBoss.RaidBossParty.IceFairySirra":                                                 nil,
		"script.ai.individual.Monster.RaidBoss.RaidBossParty.RaidBossParty":                                                 nil,
		"script.ai.individual.Monster.RaidBoss.RaidBossParty.RaidBossType2":                                                 nil,
		"script.ai.individual.Monster.RaidBoss.RaidBossParty.RaidBossType3":                                                 nil,
		"script.ai.individual.Monster.RaidBoss.RaidBossParty.RaidBossType3AnaisAggressive":                                  nil,
		"script.ai.individual.Monster.RaidBoss.RaidBossParty.RaidBossType4":                                                 nil,
		"script.ai.individual.Monster.RaidBoss.RaidBossParty.RaidBossType4AndreasAggressive":                                nil,
		"script.ai.individual.Monster.RaidBoss.RaidBossParty.RaidBossType5":                                                 nil,
		"script.ai.individual.Monster.RaidBoss.RaidBossStandard":                                                            nil,
		"script.ai.individual.Monster.RaidPrivate.RaidPrivateArcher":                                                        nil,
		"script.ai.individual.Monster.RaidPrivate.RaidPrivateFighter":                                                       nil,
		"script.ai.individual.Monster.RaidPrivate.RaidPrivateHealer":                                                        nil,
		"script.ai.individual.Monster.RaidPrivate.RaidPrivateWizard":                                                        nil,
		"script.ai.individual.Monster.WarriorDDMagicHold.WarriorDDMagicHold":                                                nil,
		"script.ai.individual.Monster.WarriorDDMagicHold.WarriorDDMagicHoldAggressive.ValakasPrivateLavasaurus":             nil,
		"script.ai.individual.Monster.WarriorDDMagicHold.WarriorDDMagicHoldAggressive.WarriorDDMagicHoldAggressive":         nil,
		"script.ai.individual.Monster.WarriorHero":                                                                          monster.WarriorHero,
	}
}
