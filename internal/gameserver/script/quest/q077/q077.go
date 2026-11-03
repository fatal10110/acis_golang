// Package q077 is the quest Saga of the Dominator: the third class
// change of an Overlord, on the saga core.
package q077

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/thirdclass"
)

// dominator is the class the saga leads to.
const dominator = 115

// New returns the saga, registered in f.
func New(f *thirdclass.Family) script.Script { return thirdclass.New(f, Saga()) }

// Saga returns what the saga sets on the core.
func Saga() thirdclass.Saga {
	return thirdclass.Saga{
		Name:    "Q077_SagaOfTheDominator",
		QuestID: 77,
		Title:   "Saga of the Dominator",
		ClassID: dominator,

		ItemMain:        7100,
		ItemOptional:    0,
		ItemReward:      7539,
		ItemAmulet1st:   7275,
		ItemAmulet2nd:   7306,
		ItemAmulet3rd:   7337,
		ItemAmulet4th:   7368,
		ItemHalishaMark: 7492,
		ItemAmulet5th:   7399,
		ItemAmulet6th:   7430,

		NPCMain:      31336,
		NPC1st:       31371,
		NPC2nd:       31624,
		NPCTablet1st: 31646,
		NPCTablet2nd: 31648,
		NPCTablet3rd: 31653,
		NPC3rd:       31290,
		NPCTablet4th: 31654,
		NPC4th:       31290,
		NPCTablet5th: 31655,
		NPCTablet6th: 31656,
		NPCDefender:  31636,

		MobGuardian:  27215,
		MobCorrupted: 27294,
		MobHalisha:   27226,
		MobAttacker:  27262,

		LocCorrupted: script.Loc{X: 162898, Y: -76492, Z: -3096, Heading: -1},
		LocAttacker:  script.Loc{X: 47429, Y: -56923, Z: -2383, Heading: -1},
		LocDefender:  script.Loc{X: 47391, Y: -56929, Z: -2370, Heading: -1},

		MsgCorruptedSpawn:      7750,
		MsgCorruptedDespawn:    7751,
		MsgCorruptedKill:       7752,
		MsgHalishaSpawn:        7753,
		MsgHalishaDespawn:      7756,
		MsgHalishaKill:         7754,
		MsgHalishaKillOther:    7755,
		MsgAttackerSpawn:       7764,
		MsgAttackerDespawn:     7765,
		MsgAttackerAttack1:     7766,
		MsgAttackerAttack16:    7767,
		MsgDefenderSpawn:       7757,
		MsgDefenderDespawnWon:  7761,
		MsgDefenderDespawnLost: 7762,
		MsgDefenderCombat:      7758,
		MsgDefenderCombatIdle1: 7759,
		MsgDefenderCombatIdle2: 7760,
		MsgDefenderReward:      7763,
	}
}
