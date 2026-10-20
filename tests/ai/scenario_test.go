package ai

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/defaultnpc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/group/follower"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/group/randomteleport"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/guard"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/hall"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/individual"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/monster"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/monsterbehavior"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/royalrush"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/siegeguard"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/warrior"
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
	"script.ai.group.FollowerMovingAroundMaster":                                                     follower.New,
	"script.ai.group.RandomTeleport":                                                                 randomteleport.New,
	"script.ai.individual.AltarDoorController":                                                       individual.AltarDoorController,
	"script.ai.individual.AreadataSwitch":                                                            individual.AreadataSwitch,
	"script.ai.individual.Corpse":                                                                    individual.Corpse,
	"script.ai.individual.DaimonTheWhiteEyed":                                                        individual.DaimonTheWhiteEyed,
	"script.ai.individual.DefaultNpc":                                                                defaultnpc.New,
	"script.ai.individual.ImperialGravekeeper":                                                       individual.ImperialGravekeeper,
	"script.ai.individual.Monster.MonsterAI":                                                         monster.New,
	"script.ai.individual.Monster.WarriorBase.Warrior.Warrior":                                       warrior.Warrior,
	"script.ai.individual.Monster.WarriorBase.Warrior.WarriorCast2SkillsApproachAggressive":          warrior.Cast2SkillsApproachAggressive,
	"script.ai.individual.Monster.WarriorBase.Warrior.WarriorTimerBomb":                              warrior.TimerBomb,
	"script.ai.individual.Monster.WarriorBase.Warrior.WarriorAggressive.WarriorAggressive":           warrior.Aggressive,
	"script.ai.individual.Monster.WarriorBase.Warrior.WarriorAggressive.Medusa":                      warrior.Medusa,
	"script.ai.individual.Monster.WarriorBase.Warrior.RoyalRushBomb.RoyalRushBomb":                   warrior.RoyalRushBomb,
	"script.ai.individual.Monster.WarriorBase.Warrior.RoyalRushBomb.RoyalRushHereticBomb":            warrior.RoyalRushHereticBomb,
	"script.ai.individual.Monster.WarriorBase.Warrior.RoyalRushBomb.RoyalRushHereticBomb2":           warrior.RoyalRushHereticBomb2,
	"script.ai.individual.Monster.WarriorBase.Warrior.RoyalRushRoomBossBasic.RoyalRushRoomBossBasic": warrior.RoyalRushRoomBossBasic,
	"script.ai.individual.Monster.WarriorBase.Warrior.RoyalRushRoomBossBasic.RoyalRushRoomBoss1":     warrior.RoyalRushRoomBoss1,
	"script.ai.individual.Monster.WarriorBase.Warrior.RoyalRushRoomBossBasic.RoyalRushRoomBoss4":     warrior.RoyalRushRoomBoss4,
	"script.ai.individual.Monster.WarriorBase.Warrior.TyrannoPrimeval.TyrannoPrimeval":               warrior.TyrannoPrimeval,
	"script.ai.individual.Monster.WarriorBase.WarriorBase":                                           warrior.Base,
	"script.ai.individual.Monster.WarriorHero":                                                       monster.WarriorHero,
	"script.ai.individual.Guard.Guard":                                                               guard.Guard,
	"script.ai.individual.Guard.GuardFixed":                                                          guard.Fixed,
	"script.ai.individual.Guard.GuardMoveAround":                                                     guard.MoveAround,
	"script.ai.individual.Guard.GuardMoveAroundFixed":                                                guard.MoveAroundFixed,
	"script.ai.individual.Guard.GuardStand":                                                          guard.Stand,
	"script.ai.individual.RoyalRushDefaultNpc.RoyalRushAfflict":                                      royalrush.Afflict,
	"script.ai.individual.RoyalRushDefaultNpc.RoyalRushDefaultNpc":                                   royalrush.DefaultNpc,
	"script.ai.individual.RoyalRushDefaultNpc.RoyalRushTrasureBox":                                   royalrush.TreasureBox,
	"script.ai.individual.RoyalRushMissionNpc":                                                       individual.RoyalRushMissionNpc,
	"script.ai.individual.WarriorPetForPC":                                                           individual.WarriorPetForPC,
	"script.ai.siegablehall.Agit02Dietrich":                                                          hall.Dietrich,
	"script.ai.siegablehall.Agit02Gustav":                                                            hall.Gustav,
	"script.ai.siegablehall.Agit02Mikhail":                                                           hall.Mikhail,
	"script.ai.siegablehall.Agit03Alfred":                                                            hall.Alfred,
	"script.ai.siegablehall.Agit03Giselle":                                                           hall.Giselle,
	"script.ai.siegablehall.Agit03Lidia":                                                             hall.Lidia,
	"script.ai.siegeguards.GludioCleric":                                                             siegeguard.Cleric,
	"script.ai.siegeguards.GludioClericStand":                                                        siegeguard.ClericStand,
	"script.ai.siegeguards.GludioHold.GludioBowGuard":                                                siegeguard.BowGuard,
	"script.ai.siegeguards.GludioHold.GludioCourtGuard":                                              siegeguard.CourtGuard,
	"script.ai.siegeguards.GludioHold.GludioDukeLewinWaldner":                                        siegeguard.DukeLewinWaldner,
	"script.ai.siegeguards.GludioHold.GludioHold":                                                    siegeguard.Hold,
	"script.ai.siegeguards.GludioHold.GludioKnight":                                                  siegeguard.Knight,
	"script.ai.siegeguards.GludioHold.GludioSwordGuard":                                              siegeguard.SwordGuard,
	"script.ai.siegeguards.GludioHold.GludioSwordGuardUseSkill":                                      siegeguard.SwordGuardUseSkill,
	"script.ai.siegeguards.GludioWizardStand":                                                        siegeguard.WizardStand,
	"script.ai.siegeguards.GludioWizard.GludioWizard":                                                siegeguard.Wizard,
	"script.ai.siegeguards.GludioWizard.GludioWizardUseSkill":                                        siegeguard.WizardUseSkill,

	// The MonsterBehavior chain.
	"script.ai.individual.Monster.MonsterBehavior.MonsterBehavior":                              monsterbehavior.MonsterBehavior,
	"script.ai.individual.Monster.MonsterBehavior.WizardBehavior.WizardBehavior":                monsterbehavior.WizardBehavior,
	"script.ai.individual.Monster.MonsterBehavior.WizardBehavior.WizardUseSkill.WizardUseSkill": monsterbehavior.WizardUseSkill,
}

// TestScenarios plays every NPC behavior scenario of testdata/scenarios
// through the booted server and MariaDB (format: package scenario).
func TestScenarios(t *testing.T) {
	t.Parallel()
	scenario.RunDir(t, "testdata/scenarios", scenario.Config{Catalog: scenarioScripts})
}
