// Package q078 is the quest Saga of the Doomcryer: the third class
// change of a Warcryer, on the saga core.
package q078

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/thirdclass"
)

// doomCryer is the class the saga leads to.
const doomCryer = 116

// New returns the saga, registered in f.
func New(f *thirdclass.Family) script.Script { return thirdclass.New(f, Saga()) }

// Saga returns what the saga sets on the core.
func Saga() thirdclass.Saga {
	return thirdclass.Saga{
		Name:    "Q078_SagaOfTheDoomcryer",
		QuestID: 78,
		Title:   "Saga of the Doomcryer",
		ClassID: doomCryer,

		ItemMain:        7101,
		ItemOptional:    0,
		ItemReward:      7539,
		ItemAmulet1st:   7276,
		ItemAmulet2nd:   7307,
		ItemAmulet3rd:   7338,
		ItemAmulet4th:   7369,
		ItemHalishaMark: 7493,
		ItemAmulet5th:   7400,
		ItemAmulet6th:   7431,

		NPCMain:      31336,
		NPC1st:       31589,
		NPC2nd:       31624,
		NPCTablet1st: 31646,
		NPCTablet2nd: 31649,
		NPCTablet3rd: 31650,
		NPC3rd:       31290,
		NPCTablet4th: 31654,
		NPC4th:       31290,
		NPCTablet5th: 31655,
		NPCTablet6th: 31657,
		NPCDefender:  31642,

		MobGuardian:  27216,
		MobCorrupted: 27295,
		MobHalisha:   27227,
		MobAttacker:  27285,

		LocCorrupted: script.Loc{X: 191046, Y: -40640, Z: -3042, Heading: -1},
		LocAttacker:  script.Loc{X: 46087, Y: -36372, Z: -1685, Heading: -1},
		LocDefender:  script.Loc{X: 46066, Y: -36396, Z: -1685, Heading: -1},

		MsgCorruptedSpawn:      7850,
		MsgCorruptedDespawn:    7851,
		MsgCorruptedKill:       7852,
		MsgHalishaSpawn:        7853,
		MsgHalishaDespawn:      7856,
		MsgHalishaKill:         7854,
		MsgHalishaKillOther:    7855,
		MsgAttackerSpawn:       7864,
		MsgAttackerDespawn:     7865,
		MsgAttackerAttack1:     7866,
		MsgAttackerAttack16:    7867,
		MsgDefenderSpawn:       7857,
		MsgDefenderDespawnWon:  7861,
		MsgDefenderDespawnLost: 7862,
		MsgDefenderCombat:      7858,
		MsgDefenderCombatIdle1: 7859,
		MsgDefenderCombatIdle2: 7860,
		MsgDefenderReward:      7863,
	}
}
