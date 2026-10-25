package ai

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/agitwarrior"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/boss/core"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/boss/orfen"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/boss/queenant"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/boss/sailren"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/group/follower"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/group/randomteleport"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/guard"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/hall"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/individual"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/monster"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/monsterbehavior"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/raid"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/royalrush"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/siegeguard"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/warrior"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/wizard"
	"github.com/fatal10110/acis_golang/internal/testsupport/scenario"
)

// probeID and probeFolkID are the NPC ids the runner's probe behavior is
// bound to: a monster and a civilian, each walking when its npc line says
// walks.
const (
	probeID     = 20120
	probeFolkID = 35600
)

// probe is a behavior that answers its NPC's attacked and dying hooks with
// a sound to the attacker or killer, naming the hook and the damage, its
// see-creature hook with a sound to the player seen, its created hook with
// a chat line, and its see-item and picked-item hooks with a chat line
// naming the items, a see-item hook asking it to pick each one up, so a
// scenario sees through packets which hooks its hit, kill, approach and
// look steps and its NPCs' spawns raised.
// It stands in for the behavior scripts until the first one is ported.
func probe() script.Script {
	sound := func(s *script.Script, c script.Creature, name string) {
		if p, ok := c.(*script.Player); ok {
			s.PlaySound(p, name)
		}
	}
	return script.Script{Behavior: true, NPCs: []int32{probeID, probeFolkID}, Hooks: script.Hooks{
		OnAttacked: func(s *script.Script, e script.Attacked) {
			sound(s, e.Attacker, fmt.Sprintf("Probe.attacked_%d", e.Damage))
		},
		OnMyDying:     func(s *script.Script, e script.MyDying) { sound(s, e.Killer, "Probe.dying") },
		OnSeeCreature: func(s *script.Script, e script.SeeCreature) { sound(s, e.Creature, "Probe.see") },
		OnCreated:     func(_ *script.Script, e script.Created) { e.NPC.Say("Probe.created") },
		OnSeeItem: func(_ *script.Script, e script.SeeItem) {
			ids := make([]string, 0, len(e.Items))
			for _, it := range e.Items {
				ids = append(ids, strconv.Itoa(int(it.ItemID)))
			}
			slices.Sort(ids)
			e.NPC.Say(fmt.Sprintf("Probe.see_item q=%d %s", e.Quantity, strings.Join(ids, ",")))
			for _, it := range e.Items {
				e.NPC.AddPickUpDesire(it.ObjectID, 1_000_000)
			}
		},
		OnPickedItem: func(_ *script.Script, e script.PickedItem) {
			e.NPC.Say(fmt.Sprintf("Probe.picked %d", e.Item.ItemID))
		},
	}}
}

// scenarioScripts are the scripts an NPC behavior scenario may list, by
// scripts.xml path.
var scenarioScripts = script.Catalog{
	"ai.Probe": probe,
	"script.ai.group.FollowerMovingAroundMaster":                                                                            follower.New,
	"script.ai.group.RandomTeleport":                                                                                        randomteleport.New,
	"script.ai.individual.AgitWarrior.AgitWarrior":                                                                          agitwarrior.Warrior,
	"script.ai.individual.AgitWarrior.AgitWarriorFlee.AgitThief":                                                            agitwarrior.Thief,
	"script.ai.individual.AgitWarrior.AgitWarriorFlee.AgitWarriorFlee":                                                      agitwarrior.WarriorFlee,
	"script.ai.individual.AgitWarrior.AgitWarriorFlee.AgitWizardFlee":                                                       agitwarrior.WizardFlee,
	"script.ai.individual.AgitWarrior.AgitWizard":                                                                           agitwarrior.Wizard,
	"script.ai.individual.AltarDoorController":                                                                              individual.AltarDoorController,
	"script.ai.individual.AreadataSwitch":                                                                                   individual.AreadataSwitch,
	"script.ai.individual.Corpse":                                                                                           individual.Corpse,
	"script.ai.individual.DaimonTheWhiteEyed":                                                                               individual.DaimonTheWhiteEyed,
	"script.ai.individual.DefaultNpc":                                                                                       defaultnpc.New,
	"script.ai.individual.ImperialGravekeeper":                                                                              individual.ImperialGravekeeper,
	"script.ai.individual.Monster.MonsterAI":                                                                                monster.New,
	"script.ai.individual.Monster.WarriorBase.Warrior.Warrior":                                                              warrior.Warrior,
	"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCast2SkillsApproachAggressive":                                 warrior.Cast2SkillsApproachAggressive,
	"script.ai.individual.Monster.WarriorBase.Warrior.WarriorTimerBomb":                                                     warrior.TimerBomb,
	"script.ai.individual.Monster.WarriorBase.Warrior.WarriorHereticPriestAggressive":                                       warrior.HereticPriestAggressive,
	"script.ai.individual.Monster.WizardBase.WizardBase":                                                                    wizard.Base,
	"script.ai.individual.Monster.WizardBase.Wizard.Wizard":                                                                 wizard.Wizard,
	"script.ai.individual.Monster.WizardBase.Wizard.WizardHereticPriestAggressive":                                          wizard.HereticPriestAggressive,
	"script.ai.individual.Monster.WarriorBase.Warrior.WarriorAggressive.WarriorAggressive":                                  warrior.Aggressive,
	"script.ai.individual.Monster.WarriorBase.Warrior.WarriorAggressive.Medusa":                                             warrior.Medusa,
	"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCast3SkillsApproach.WarriorCast3SkillsApproach":                warrior.Cast3SkillsApproach,
	"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCast3SkillsMagical.WarriorCast3SkillsMagical":                  warrior.Cast3SkillsMagical,
	"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCast3SkillsMagical.WarriorCast3SkillsCurse":                    warrior.Cast3SkillsCurse,
	"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCast3SkillsMagical2.WarriorCast3SkillsMagical2":                warrior.Cast3SkillsMagical2,
	"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCast3SkillsMagical2.WarriorCast3SkillsMagical2Aggressive":      warrior.Cast3SkillsMagical2Aggressive,
	"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCast3SkillsMagical2.WarriorCast3SkillsMagical2StoneAggressive": warrior.Cast3SkillsMagical2StoneAggressive,
	"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCast3SkillsMagical3.WarriorCast3SkillsMagical3":                warrior.Cast3SkillsMagical3,
	"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCast3SkillsMagical4.WarriorCast3SkillsMagical4":                warrior.Cast3SkillsMagical4,
	"script.ai.individual.Monster.WarriorBase.Warrior.WarriorGrowth.WarriorGrowthStep4Aggressive":                           warrior.WarriorGrowthStep4Aggressive,
	"script.ai.individual.Monster.WarriorBase.WarriorBase":                                                                  warrior.Base,
	"script.ai.individual.Monster.WarriorHero":                                                                              monster.WarriorHero,
	"script.ai.individual.Guard.Guard":                                                                                      guard.Guard,
	"script.ai.individual.Guard.GuardFixed":                                                                                 guard.Fixed,
	"script.ai.individual.Guard.GuardMoveAround":                                                                            guard.MoveAround,
	"script.ai.individual.Guard.GuardMoveAroundFixed":                                                                       guard.MoveAroundFixed,
	"script.ai.individual.Guard.GuardStand":                                                                                 guard.Stand,
	"script.ai.individual.RoyalRushDefaultNpc.RoyalRushAfflict":                                                             royalrush.Afflict,
	"script.ai.individual.RoyalRushDefaultNpc.RoyalRushDefaultNpc":                                                          royalrush.DefaultNpc,
	"script.ai.individual.RoyalRushDefaultNpc.RoyalRushKeyBox":                                                              royalrush.KeyBox,
	"script.ai.individual.RoyalRushDefaultNpc.RoyalRushTrasureBox":                                                          royalrush.TreasureBox,
	"script.ai.individual.RoyalRushMissionNpc":                                                                              individual.RoyalRushMissionNpc,
	"script.ai.individual.WarriorPetForPC":                                                                                  individual.WarriorPetForPC,
	"script.ai.siegablehall.Agit02Dietrich":                                                                                 hall.Dietrich,
	"script.ai.siegablehall.Agit02Gustav":                                                                                   hall.Gustav,
	"script.ai.siegablehall.Agit02Mikhail":                                                                                  hall.Mikhail,
	"script.ai.siegablehall.Agit03Alfred":                                                                                   hall.Alfred,
	"script.ai.siegablehall.Agit03Giselle":                                                                                  hall.Giselle,
	"script.ai.siegablehall.Agit03Lidia":                                                                                    hall.Lidia,
	"script.ai.siegablehall.AzitWateringCrazyYeti":                                                                          hall.CrazyYeti,
	"script.ai.siegablehall.AzitWateringGroud":                                                                              hall.Gourd,
	"script.ai.siegablehall.AzitWateringMimic":                                                                              hall.Mimic,
	"script.ai.siegablehall.AzitWateringMimicMagical":                                                                       hall.MimicMagical,
	"script.ai.siegablehall.AzitWateringRecycleCan":                                                                         hall.RecycleCan,
	"script.ai.siegablehall.AzitWateringTreasureBox":                                                                        hall.TreasureBox,
	"script.ai.siegablehall.Agit01Move1.Agit01Move1":                                                                        hall.Move1,
	"script.ai.siegablehall.Agit01Move1.Agit01PartisanSorcerer2":                                                            hall.PartisanSorcerer2,
	"script.ai.siegablehall.Agit01PartisanHealer1.Agit01PartisanHealer1":                                                    hall.PartisanHealer1,
	"script.ai.siegablehall.Agit01Stand1.Agit01Stand1":                                                                      hall.Stand1,
	"script.ai.siegablehall.BloodyLordNurka1.BloodyLordNurka1":                                                              hall.Nurka1,
	"script.ai.siegeguards.GludioCleric":                                                                                    siegeguard.Cleric,
	"script.ai.siegeguards.GludioClericStand":                                                                               siegeguard.ClericStand,
	"script.ai.siegeguards.GludioHold.GludioBowGuard":                                                                       siegeguard.BowGuard,
	"script.ai.siegeguards.GludioHold.GludioCourtGuard":                                                                     siegeguard.CourtGuard,
	"script.ai.siegeguards.GludioHold.GludioDukeLewinWaldner":                                                               siegeguard.DukeLewinWaldner,
	"script.ai.siegeguards.GludioHold.GludioHold":                                                                           siegeguard.Hold,
	"script.ai.siegeguards.GludioHold.GludioKnight":                                                                         siegeguard.Knight,
	"script.ai.siegeguards.GludioHold.GludioSwordGuard":                                                                     siegeguard.SwordGuard,
	"script.ai.siegeguards.GludioHold.GludioSwordGuardUseSkill":                                                             siegeguard.SwordGuardUseSkill,
	"script.ai.siegeguards.GludioWizardStand":                                                                               siegeguard.WizardStand,
	"script.ai.siegeguards.GludioWizard.GludioWizard":                                                                       siegeguard.Wizard,
	"script.ai.siegeguards.GludioWizard.GludioWizardUseSkill":                                                               siegeguard.WizardUseSkill,

	// The Four Sepulchers bombs and room bosses, and the tyrannosaurus.
	"script.ai.individual.Monster.WarriorBase.Warrior.RoyalRushBomb.RoyalRushBomb":                                                         warrior.RoyalRushBomb,
	"script.ai.individual.Monster.WarriorBase.Warrior.RoyalRushBomb.RoyalRushHereticBomb":                                                  warrior.RoyalRushHereticBomb,
	"script.ai.individual.Monster.WarriorBase.Warrior.RoyalRushBomb.RoyalRushHereticBomb2":                                                 warrior.RoyalRushHereticBomb2,
	"script.ai.individual.Monster.WarriorBase.Warrior.RoyalRushRoomBossBasic.RoyalRushRoomBossBasic":                                       warrior.RoyalRushRoomBossBasic,
	"script.ai.individual.Monster.WarriorBase.Warrior.RoyalRushRoomBossBasic.RoyalRushRoomBoss1":                                           warrior.RoyalRushRoomBoss1,
	"script.ai.individual.Monster.WarriorBase.Warrior.RoyalRushRoomBossBasic.RoyalRushRoomBoss4":                                           warrior.RoyalRushRoomBoss4,
	"script.ai.individual.Monster.WarriorBase.Warrior.TyrannoPrimeval.TyrannoPrimeval":                                                     warrior.TyrannoPrimeval,
	"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderWarrior":                                               warrior.PartyLeader,
	"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderCoupleCaptain.PartyLeaderCoupleCaptain":                warrior.PartyLeaderCoupleCaptain,
	"script.ai.individual.Monster.WarriorBase.Warrior.PartyLeaderWarrior.PartyLeaderCoupleCaptain.PartyLeaderCoupleCaptainNurseAggressive": warrior.PartyLeaderCoupleCaptainNurseAggressive,

	// The DD magic physical specials and the physical poisons.
	"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastDDMagicPhysicalSpecial.WarriorCastDDMagicPhysicalSpecial":                warrior.CastDDMagicPhysicalSpecial,
	"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastDDMagicPhysicalSpecial.WarriorCastDDMagicPhysicalSpecialBuffAggressive":  warrior.CastDDMagicPhysicalSpecialBuffAggressive,
	"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastEnchantPhysicalPoison.WarriorCastEnchantPhysicalPoison":                  warrior.CastEnchantPhysicalPoison,
	"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastEnchantPhysicalPoison.WarriorCastEnchantPhysicalPoisonAggressive":        warrior.CastEnchantPhysicalPoisonAggressive,
	"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastEnchantPhysicalPoison.WarriorCastDDMagicEnchantPhysicalPoisonAggressive": warrior.CastDDMagicEnchantPhysicalPoisonAggressive,

	// The hold and splash casters.
	"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastHoldMagic.WarriorCastHoldMagic": warrior.CastHoldMagic,
	"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastSplash.WarriorCastSplash":       warrior.CastSplash,
	"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastSplash.Agit02DoomTrooperAgit":   warrior.Agit02DoomTrooperAgit,

	// The summon, lab clan and heal casters.
	"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastSummonPC.WarriorCastSummonPC":                                                         warrior.CastSummonPC,
	"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingEnchantLabClan1.WarriorCastingEnchantLabClan1":                                     warrior.CastingEnchantLabClan1,
	"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingEnchantLabClan1.WarriorCastingEnchantLabClan2.WarriorCastingEnchantLabClan2":       warrior.CastingEnchantLabClan2,
	"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingEnchantLabClan1.WarriorCastingEnchantLabClan2.WarriorPhysicalSpecialLabAggressive": warrior.PhysicalSpecialLabAggressive,
	"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingHeal.WarriorCastingHeal":                                                           warrior.CastingHeal,

	// The sleeping healer and the corpse zombies.
	"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCastingHeal.WarriorCastingHealSleep":             warrior.CastingHealSleep,
	"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCorpseZombie.WarriorCorpseZombie":                warrior.CorpseZombie,
	"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCorpseZombie.WarriorCorpseZombieDDMagic":         warrior.CorpseZombieDDMagic,
	"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCorpseZombie.WarriorCorpseZombiePhysicalSpecial": warrior.CorpseZombiePhysicalSpecial,

	// The MonsterBehavior chain.
	"script.ai.individual.Monster.MonsterBehavior.MonsterBehavior":                                 monsterbehavior.MonsterBehavior,
	"script.ai.individual.Monster.MonsterBehavior.WizardBehavior.WizardBehavior":                   monsterbehavior.WizardBehavior,
	"script.ai.individual.Monster.MonsterBehavior.WizardBehavior.WizardUseSkill.WizardUseSkill":    monsterbehavior.WizardUseSkill,
	"script.ai.individual.Monster.MonsterBehavior.WarriorBehavior.WarriorBehavior":                 monsterbehavior.WarriorBehavior,
	"script.ai.individual.Monster.MonsterBehavior.WarriorBehavior.WarriorUseSkill.WarriorUseSkill": monsterbehavior.WarriorUseSkill,

	// The raid bosses.
	"script.ai.individual.Monster.RaidBoss.RaidBossStandard":                                                            raid.BossStandard,
	"script.ai.individual.Monster.RaidBoss.RaidBossAlone.RaidBossType1.RaidBossType1":                                   raid.BossType1,
	"script.ai.individual.Monster.RaidBoss.RaidBossAlone.RaidBossType1.Gordon":                                          raid.Gordon,
	"script.ai.individual.Monster.RaidBoss.RaidBossAlone.RaidBossType1.RaidBossAloneSummonPrivate":                      raid.BossAloneSummonPrivate,
	"script.ai.individual.Monster.RaidBoss.RaidBossAlone.RaidBossType1.RaidBossForTeleportDungeon":                      raid.BossForTeleportDungeon,
	"script.ai.individual.Monster.RaidBoss.RaidBossAlone.RaidBossType1.Sailren":                                         raid.Sailren,
	"script.ai.individual.Monster.RaidBoss.RaidBossAlone.RaidBossType1.RaidBossType1Aggressive.DrChaosGiganticGolem":    raid.DrChaosGiganticGolem,
	"script.ai.individual.Monster.RaidBoss.RaidBossAlone.RaidBossType1.RaidBossType1Aggressive.FreyaRoyalGuard":         raid.FreyaRoyalGuard,
	"script.ai.individual.Monster.RaidBoss.RaidBossAlone.RaidBossType1.RaidBossType1Aggressive.RaidBossType1Aggressive": raid.BossType1Aggressive,
	"script.ai.individual.Monster.RaidBoss.RaidBossParty.RaidBossParty":                                                 raid.BossParty,
	"script.ai.individual.Monster.RaidBoss.RaidBossParty.RaidBossType2":                                                 raid.BossType2,
	"script.ai.individual.Monster.RaidBoss.RaidBossParty.IceFairySirra":                                                 raid.IceFairySirra,
	"script.ai.individual.Monster.RaidBoss.RaidBossParty.RaidBossType3":                                                 raid.BossType3,
	"script.ai.individual.Monster.RaidBoss.RaidBossParty.RaidBossType3AnaisAggressive":                                  raid.BossType3AnaisAggressive,
	"script.ai.individual.Monster.RaidBoss.RaidBossParty.RaidBossType4":                                                 raid.BossType4,

	// The grand bosses.
	"script.ai.boss.core.Core":                        core.Core,
	"script.ai.boss.queenant.QueenAnt":                queenant.QueenAnt,
	"script.ai.boss.queenant.QueenAntPrivateAntLarva": queenant.Larva,
	"script.ai.boss.queenant.QueenAntPrivateNurseAnt": queenant.Nurse,
	"script.ai.boss.orfen.Orfen":                      orfen.Orfen,
	"script.ai.boss.orfen.OrfenPrivateRibaIren":       orfen.RibaIren,
	"script.ai.boss.sailren.SailrenDummy":             sailren.SailrenDummy,
	"script.ai.boss.sailren.StatueOfShilen":           sailren.StatueOfShilen,
}

// TestScenarios plays every NPC behavior scenario of testdata/scenarios
// through the booted server and MariaDB (format: package scenario).
func TestScenarios(t *testing.T) {
	t.Parallel()
	scenario.RunDir(t, "testdata/scenarios", scenario.Config{Catalog: scenarioScripts})
}
