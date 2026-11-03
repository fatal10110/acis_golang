// Package q095 is the quest Saga of the Hell Knight: the third class
// change of a Dark Avenger, on the saga core.
package q095

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/thirdclass"
)

// hellKnight is the class the saga leads to.
const hellKnight = 91

// New returns the saga, registered in f.
func New(f *thirdclass.Family) script.Script { return thirdclass.New(f, Saga()) }

// Saga returns what the saga sets on the core.
func Saga() thirdclass.Saga {
	return thirdclass.Saga{
		Name:    "Q095_SagaOfTheHellKnight",
		QuestID: 95,
		Title:   "Saga of the Hell Knight",
		ClassID: hellKnight,

		ItemMain:        7086,
		ItemOptional:    0,
		ItemReward:      7532,
		ItemAmulet1st:   7293,
		ItemAmulet2nd:   7324,
		ItemAmulet3rd:   7355,
		ItemAmulet4th:   7386,
		ItemHalishaMark: 7510,
		ItemAmulet5th:   7417,
		ItemAmulet6th:   7448,

		NPCMain:      31582,
		NPC1st:       31297,
		NPC2nd:       31623,
		NPCTablet1st: 31646,
		NPCTablet2nd: 31647,
		NPCTablet3rd: 31653,
		NPC3rd:       31297,
		NPCTablet4th: 31654,
		NPC4th:       31297,
		NPCTablet5th: 31655,
		NPCTablet6th: 31656,
		NPCDefender:  31599,

		MobGuardian:  27214,
		MobCorrupted: 27258,
		MobHalisha:   27244,
		MobAttacker:  27263,

		LocCorrupted: script.Loc{X: 162898, Y: -76492, Z: -3096, Heading: -1},
		LocAttacker:  script.Loc{X: 47391, Y: -56929, Z: -2370, Heading: -1},
		LocDefender:  script.Loc{X: 47429, Y: -56923, Z: -2383, Heading: -1},

		MsgCorruptedSpawn:      9550,
		MsgCorruptedDespawn:    9551,
		MsgCorruptedKill:       9552,
		MsgHalishaSpawn:        9553,
		MsgHalishaDespawn:      9556,
		MsgHalishaKill:         9554,
		MsgHalishaKillOther:    9555,
		MsgAttackerSpawn:       9564,
		MsgAttackerDespawn:     9565,
		MsgAttackerAttack1:     9566,
		MsgAttackerAttack16:    9567,
		MsgDefenderSpawn:       9557,
		MsgDefenderDespawnWon:  9561,
		MsgDefenderDespawnLost: 9562,
		MsgDefenderCombat:      9558,
		MsgDefenderCombatIdle1: 9559,
		MsgDefenderCombatIdle2: 9560,
		MsgDefenderReward:      9563,
	}
}
