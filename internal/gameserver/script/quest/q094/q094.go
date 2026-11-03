// Package q094 is the quest Saga of the Soultaker: the third class
// change of a Necromancer, on the saga core.
package q094

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/thirdclass"
)

// soultaker is the class the saga leads to.
const soultaker = 95

// New returns the saga, registered in f.
func New(f *thirdclass.Family) script.Script { return thirdclass.New(f, Saga()) }

// Saga returns what the saga sets on the core.
func Saga() thirdclass.Saga {
	return thirdclass.Saga{
		Name:    "Q094_SagaOfTheSoultaker",
		QuestID: 94,
		Title:   "Saga of the Soultaker",
		ClassID: soultaker,

		ItemMain:        7085,
		ItemOptional:    0,
		ItemReward:      7533,
		ItemAmulet1st:   7292,
		ItemAmulet2nd:   7323,
		ItemAmulet3rd:   7354,
		ItemAmulet4th:   7385,
		ItemHalishaMark: 7509,
		ItemAmulet5th:   7416,
		ItemAmulet6th:   7447,

		NPCMain:      30832,
		NPC1st:       31279,
		NPC2nd:       31623,
		NPCTablet1st: 31646,
		NPCTablet2nd: 31648,
		NPCTablet3rd: 31650,
		NPC3rd:       31279,
		NPCTablet4th: 31654,
		NPC4th:       31279,
		NPCTablet5th: 31655,
		NPCTablet6th: 31657,
		NPCDefender:  31645,

		MobGuardian:  27215,
		MobCorrupted: 27257,
		MobHalisha:   27243,
		MobAttacker:  27265,

		LocCorrupted: script.Loc{X: 191046, Y: -40640, Z: -3042, Heading: -1},
		LocAttacker:  script.Loc{X: 46066, Y: -36396, Z: -1685, Heading: -1},
		LocDefender:  script.Loc{X: 46087, Y: -36372, Z: -1685, Heading: -1},

		MsgCorruptedSpawn:      9450,
		MsgCorruptedDespawn:    9451,
		MsgCorruptedKill:       9452,
		MsgHalishaSpawn:        9453,
		MsgHalishaDespawn:      9456,
		MsgHalishaKill:         9454,
		MsgHalishaKillOther:    9455,
		MsgAttackerSpawn:       9464,
		MsgAttackerDespawn:     9465,
		MsgAttackerAttack1:     9466,
		MsgAttackerAttack16:    9467,
		MsgDefenderSpawn:       9457,
		MsgDefenderDespawnWon:  9461,
		MsgDefenderDespawnLost: 9462,
		MsgDefenderCombat:      9458,
		MsgDefenderCombatIdle1: 9459,
		MsgDefenderCombatIdle2: 9460,
		MsgDefenderReward:      9463,
	}
}
