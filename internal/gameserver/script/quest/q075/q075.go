// Package q075 is the quest Saga of the Titan: the third class
// change of a Destroyer, on the saga core.
package q075

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/thirdclass"
)

// titan is the class the saga leads to.
const titan = 113

// New returns the saga, registered in f.
func New(f *thirdclass.Family) script.Script { return thirdclass.New(f, Saga()) }

// Saga returns what the saga sets on the core.
func Saga() thirdclass.Saga {
	return thirdclass.Saga{
		Name:    "Q075_SagaOfTheTitan",
		QuestID: 75,
		Title:   "Saga of the Titan",
		ClassID: titan,

		ItemMain:        7098,
		ItemOptional:    0,
		ItemReward:      7539,
		ItemAmulet1st:   7273,
		ItemAmulet2nd:   7304,
		ItemAmulet3rd:   7335,
		ItemAmulet4th:   7366,
		ItemHalishaMark: 7490,
		ItemAmulet5th:   7397,
		ItemAmulet6th:   7428,

		NPCMain:      31327,
		NPC1st:       31289,
		NPC2nd:       31624,
		NPCTablet1st: 31646,
		NPCTablet2nd: 31649,
		NPCTablet3rd: 31651,
		NPC3rd:       31290,
		NPCTablet4th: 31654,
		NPC4th:       31290,
		NPCTablet5th: 31655,
		NPCTablet6th: 31658,
		NPCDefender:  31607,

		MobGuardian:  27216,
		MobCorrupted: 27292,
		MobHalisha:   27224,
		MobAttacker:  27283,

		LocCorrupted: script.Loc{X: 119518, Y: -28658, Z: -3811, Heading: -1},
		LocAttacker:  script.Loc{X: 181215, Y: 36676, Z: -4812, Heading: -1},
		LocDefender:  script.Loc{X: 181227, Y: 36703, Z: -4816, Heading: -1},

		MsgCorruptedSpawn:      7550,
		MsgCorruptedDespawn:    7551,
		MsgCorruptedKill:       7552,
		MsgHalishaSpawn:        7553,
		MsgHalishaDespawn:      7556,
		MsgHalishaKill:         7554,
		MsgHalishaKillOther:    7555,
		MsgAttackerSpawn:       7564,
		MsgAttackerDespawn:     7565,
		MsgAttackerAttack1:     7566,
		MsgAttackerAttack16:    7567,
		MsgDefenderSpawn:       7557,
		MsgDefenderDespawnWon:  7561,
		MsgDefenderDespawnLost: 7562,
		MsgDefenderCombat:      7558,
		MsgDefenderCombatIdle1: 7559,
		MsgDefenderCombatIdle2: 7560,
		MsgDefenderReward:      7563,
	}
}
