// Package q087 is the quest Saga of Eva's Saint: the third class
// change of an Elven Elder, on the saga core.
package q087

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/thirdclass"
)

// evasSaint is the class the saga leads to.
const evasSaint = 105

// New returns the saga, registered in f.
func New(f *thirdclass.Family) script.Script { return thirdclass.New(f, Saga()) }

// Saga returns what the saga sets on the core.
func Saga() thirdclass.Saga {
	return thirdclass.Saga{
		Name:    "Q087_SagaOfEvasSaint",
		QuestID: 87,
		Title:   "Saga of Eva's Saint",
		ClassID: evasSaint,

		ItemMain:        7088,
		ItemOptional:    0,
		ItemReward:      7524,
		ItemAmulet1st:   7285,
		ItemAmulet2nd:   7316,
		ItemAmulet3rd:   7347,
		ItemAmulet4th:   7378,
		ItemHalishaMark: 7502,
		ItemAmulet5th:   7409,
		ItemAmulet6th:   7440,

		NPCMain:      30191,
		NPC1st:       31588,
		NPC2nd:       31626,
		NPCTablet1st: 31646,
		NPCTablet2nd: 31649,
		NPCTablet3rd: 31653,
		NPC3rd:       31280,
		NPCTablet4th: 31654,
		NPC4th:       31280,
		NPCTablet5th: 31655,
		NPCTablet6th: 31657,
		NPCDefender:  31620,

		MobGuardian:  27216,
		MobCorrupted: 27266,
		MobHalisha:   27236,
		MobAttacker:  27276,

		LocCorrupted: script.Loc{X: 162898, Y: -76492, Z: -3096, Heading: -1},
		LocAttacker:  script.Loc{X: 46087, Y: -36372, Z: -1685, Heading: -1},
		LocDefender:  script.Loc{X: 46066, Y: -36396, Z: -1685, Heading: -1},

		MsgCorruptedSpawn:      8750,
		MsgCorruptedDespawn:    8751,
		MsgCorruptedKill:       8752,
		MsgHalishaSpawn:        8753,
		MsgHalishaDespawn:      8756,
		MsgHalishaKill:         8754,
		MsgHalishaKillOther:    8755,
		MsgAttackerSpawn:       8764,
		MsgAttackerDespawn:     8765,
		MsgAttackerAttack1:     8766,
		MsgAttackerAttack16:    8767,
		MsgDefenderSpawn:       8757,
		MsgDefenderDespawnWon:  8761,
		MsgDefenderDespawnLost: 8762,
		MsgDefenderCombat:      8758,
		MsgDefenderCombatIdle1: 8759,
		MsgDefenderCombatIdle2: 8760,
		MsgDefenderReward:      8763,
	}
}
