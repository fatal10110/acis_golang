// Package q070 is the quest Saga of the Phoenix Knight: the third class
// change of a Paladin, on the saga core.
package q070

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/thirdclass"
)

// phoenixKnight is the class the saga leads to.
const phoenixKnight = 90

// New returns the saga, registered in f.
func New(f *thirdclass.Family) script.Script { return thirdclass.New(f, Saga()) }

// Saga returns what the saga sets on the core.
func Saga() thirdclass.Saga {
	return thirdclass.Saga{
		Name:    "Q070_SagaOfThePhoenixKnight",
		QuestID: 70,
		Title:   "Saga of the Phoenix Knight",
		ClassID: phoenixKnight,

		ItemMain:        7093,
		ItemOptional:    6482,
		ItemReward:      7534,
		ItemAmulet1st:   7268,
		ItemAmulet2nd:   7299,
		ItemAmulet3rd:   7330,
		ItemAmulet4th:   7361,
		ItemHalishaMark: 7485,
		ItemAmulet5th:   7392,
		ItemAmulet6th:   7423,

		NPCMain:      30849,
		NPC1st:       31277,
		NPC2nd:       31624,
		NPCTablet1st: 31646,
		NPCTablet2nd: 31647,
		NPCTablet3rd: 31650,
		NPC3rd:       30849,
		NPCTablet4th: 31654,
		NPC4th:       31277,
		NPCTablet5th: 31655,
		NPCTablet6th: 31657,
		NPCDefender:  31631,

		MobGuardian:  27214,
		MobCorrupted: 27286,
		MobHalisha:   27219,
		MobAttacker:  27278,

		LocCorrupted: script.Loc{X: 191046, Y: -40640, Z: -3042, Heading: -1},
		LocAttacker:  script.Loc{X: 46087, Y: -36372, Z: -1685, Heading: -1},
		LocDefender:  script.Loc{X: 46066, Y: -36396, Z: -1685, Heading: -1},

		MsgCorruptedSpawn:      7050,
		MsgCorruptedDespawn:    7051,
		MsgCorruptedKill:       7052,
		MsgHalishaSpawn:        7053,
		MsgHalishaDespawn:      7056,
		MsgHalishaKill:         7054,
		MsgHalishaKillOther:    7055,
		MsgAttackerSpawn:       7064,
		MsgAttackerDespawn:     7065,
		MsgAttackerAttack1:     7066,
		MsgAttackerAttack16:    7067,
		MsgDefenderSpawn:       7057,
		MsgDefenderDespawnWon:  7061,
		MsgDefenderDespawnLost: 7062,
		MsgDefenderCombat:      7058,
		MsgDefenderCombatIdle1: 7059,
		MsgDefenderCombatIdle2: 7060,
		MsgDefenderReward:      7063,
	}
}
