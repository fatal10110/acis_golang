package catalog

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/monster"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/monsterbehavior"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/raid"
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
		"script.ai.individual.Monster.MonsterBehavior.WarriorBehavior.WarriorUseSkill.WarriorUseSkill":                      monsterbehavior.WarriorUseSkill,
		"script.ai.individual.Monster.MonsterBehavior.WizardBehavior.WizardUseSkill.WizardUseSkill":                         monsterbehavior.WizardUseSkill,
		"script.ai.individual.Monster.RaidBoss.RaidBossAlone.RaidBossType1.Gordon":                                          raid.Gordon,
		"script.ai.individual.Monster.RaidBoss.RaidBossAlone.RaidBossType1.RaidBossAloneSummonPrivate":                      raid.BossAloneSummonPrivate,
		"script.ai.individual.Monster.RaidBoss.RaidBossAlone.RaidBossType1.RaidBossForTeleportDungeon":                      raid.BossForTeleportDungeon,
		"script.ai.individual.Monster.RaidBoss.RaidBossAlone.RaidBossType1.RaidBossType1":                                   raid.BossType1,
		"script.ai.individual.Monster.RaidBoss.RaidBossAlone.RaidBossType1.RaidBossType1Aggressive.DrChaosGiganticGolem":    raid.DrChaosGiganticGolem,
		"script.ai.individual.Monster.RaidBoss.RaidBossAlone.RaidBossType1.RaidBossType1Aggressive.FreyaRoyalGuard":         raid.FreyaRoyalGuard,
		"script.ai.individual.Monster.RaidBoss.RaidBossAlone.RaidBossType1.RaidBossType1Aggressive.RaidBossType1Aggressive": raid.BossType1Aggressive,
		"script.ai.individual.Monster.RaidBoss.RaidBossAlone.RaidBossType1.Sailren":                                         raid.Sailren,
		"script.ai.individual.Monster.RaidBoss.RaidBossParty.IceFairySirra":                                                 raid.IceFairySirra,
		"script.ai.individual.Monster.RaidBoss.RaidBossParty.RaidBossParty":                                                 raid.BossParty,
		"script.ai.individual.Monster.RaidBoss.RaidBossParty.RaidBossType2":                                                 raid.BossType2,
		"script.ai.individual.Monster.RaidBoss.RaidBossParty.RaidBossType3":                                                 raid.BossType3,
		"script.ai.individual.Monster.RaidBoss.RaidBossParty.RaidBossType3AnaisAggressive":                                  raid.BossType3AnaisAggressive,
		"script.ai.individual.Monster.RaidBoss.RaidBossParty.RaidBossType4":                                                 raid.BossType4,
		"script.ai.individual.Monster.RaidBoss.RaidBossParty.RaidBossType4AndreasAggressive":                                raid.BossType4AndreasAggressive,
		"script.ai.individual.Monster.RaidBoss.RaidBossParty.RaidBossType5":                                                 raid.BossType5,
		"script.ai.individual.Monster.RaidBoss.RaidBossStandard":                                                            raid.BossStandard,
		"script.ai.individual.Monster.RaidPrivate.RaidPrivateArcher":                                                        raid.PrivateArcher,
		"script.ai.individual.Monster.RaidPrivate.RaidPrivateFighter":                                                       raid.PrivateFighter,
		"script.ai.individual.Monster.RaidPrivate.RaidPrivateHealer":                                                        raid.PrivateHealer,
		"script.ai.individual.Monster.RaidPrivate.RaidPrivateWizard":                                                        raid.PrivateWizard,
		"script.ai.individual.Monster.WarriorDDMagicHold.WarriorDDMagicHold":                                                monster.WarriorDDMagicHold,
		"script.ai.individual.Monster.WarriorDDMagicHold.WarriorDDMagicHoldAggressive.ValakasPrivateLavasaurus":             monster.ValakasPrivateLavasaurus,
		"script.ai.individual.Monster.WarriorDDMagicHold.WarriorDDMagicHoldAggressive.WarriorDDMagicHoldAggressive":         monster.WarriorDDMagicHoldAggressive,
		"script.ai.individual.Monster.WarriorHero":                                                                          monster.WarriorHero,
	}
}
