// Package q073 is the quest Saga of the Duelist: the third class
// change of a Gladiator, on the saga core.
package q073

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/thirdclass"
)

// duelist is the class the saga leads to.
const duelist = 88

// New returns the saga, registered in f.
func New(f *thirdclass.Family) script.Script { return thirdclass.New(f, Saga()) }

// Saga returns what the saga sets on the core.
func Saga() thirdclass.Saga {
	return thirdclass.Saga{
		Name:    "Q073_SagaOfTheDuelist",
		QuestID: 73,
		Title:   "Saga of the Duelist",
		ClassID: duelist,

		ItemMain:        7096,
		ItemOptional:    7546,
		ItemReward:      7537,
		ItemAmulet1st:   7271,
		ItemAmulet2nd:   7302,
		ItemAmulet3rd:   7333,
		ItemAmulet4th:   7364,
		ItemHalishaMark: 7488,
		ItemAmulet5th:   7395,
		ItemAmulet6th:   7426,

		NPCMain:      30849,
		NPC1st:       31226,
		NPC2nd:       31624,
		NPCTablet1st: 31646,
		NPCTablet2nd: 31647,
		NPCTablet3rd: 31653,
		NPC3rd:       31331,
		NPCTablet4th: 31654,
		NPC4th:       31277,
		NPCTablet5th: 31655,
		NPCTablet6th: 31656,
		NPCDefender:  31639,

		MobGuardian:  27214,
		MobCorrupted: 27289,
		MobHalisha:   27222,
		MobAttacker:  27281,

		LocCorrupted: script.Loc{X: 162898, Y: -76492, Z: -3096, Heading: -1},
		LocAttacker:  script.Loc{X: 47429, Y: -56923, Z: -2383, Heading: -1},
		LocDefender:  script.Loc{X: 47391, Y: -56929, Z: -2370, Heading: -1},

		MsgCorruptedSpawn:      7350,
		MsgCorruptedDespawn:    7351,
		MsgCorruptedKill:       7352,
		MsgHalishaSpawn:        7353,
		MsgHalishaDespawn:      7356,
		MsgHalishaKill:         7354,
		MsgHalishaKillOther:    7355,
		MsgAttackerSpawn:       7364,
		MsgAttackerDespawn:     7365,
		MsgAttackerAttack1:     7366,
		MsgAttackerAttack16:    7367,
		MsgDefenderSpawn:       7357,
		MsgDefenderDespawnWon:  7361,
		MsgDefenderDespawnLost: 7362,
		MsgDefenderCombat:      7358,
		MsgDefenderCombatIdle1: 7359,
		MsgDefenderCombatIdle2: 7360,
		MsgDefenderReward:      7363,
	}
}
