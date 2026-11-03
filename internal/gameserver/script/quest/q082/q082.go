// Package q082 is the quest Saga of the Sagittarius: the third class
// change of a Hawkeye, on the saga core.
package q082

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/thirdclass"
)

// sagittarius is the class the saga leads to.
const sagittarius = 92

// New returns the saga, registered in f.
func New(f *thirdclass.Family) script.Script { return thirdclass.New(f, Saga()) }

// Saga returns what the saga sets on the core.
func Saga() thirdclass.Saga {
	return thirdclass.Saga{
		Name:    "Q082_SagaOfTheSagittarius",
		QuestID: 82,
		Title:   "Saga of the Sagittarius",
		ClassID: sagittarius,

		ItemMain:        7105,
		ItemOptional:    0,
		ItemReward:      7519,
		ItemAmulet1st:   7280,
		ItemAmulet2nd:   7311,
		ItemAmulet3rd:   7342,
		ItemAmulet4th:   7373,
		ItemHalishaMark: 7497,
		ItemAmulet5th:   7404,
		ItemAmulet6th:   7435,

		NPCMain:      30702,
		NPC1st:       31604,
		NPC2nd:       31627,
		NPCTablet1st: 31646,
		NPCTablet2nd: 31647,
		NPCTablet3rd: 31650,
		NPC3rd:       31640,
		NPCTablet4th: 31654,
		NPC4th:       31641,
		NPCTablet5th: 31655,
		NPCTablet6th: 31657,
		NPCDefender:  31633,

		MobGuardian:  27214,
		MobCorrupted: 27296,
		MobHalisha:   27231,
		MobAttacker:  27305,

		LocCorrupted: script.Loc{X: 191046, Y: -40640, Z: -3042, Heading: -1},
		LocAttacker:  script.Loc{X: 46066, Y: -36396, Z: -1685, Heading: -1},
		LocDefender:  script.Loc{X: 46066, Y: -36396, Z: -1685, Heading: -1},

		MsgCorruptedSpawn:      8250,
		MsgCorruptedDespawn:    8251,
		MsgCorruptedKill:       8252,
		MsgHalishaSpawn:        8253,
		MsgHalishaDespawn:      8256,
		MsgHalishaKill:         8254,
		MsgHalishaKillOther:    8255,
		MsgAttackerSpawn:       8264,
		MsgAttackerDespawn:     8265,
		MsgAttackerAttack1:     8266,
		MsgAttackerAttack16:    8267,
		MsgDefenderSpawn:       8257,
		MsgDefenderDespawnWon:  8261,
		MsgDefenderDespawnLost: 8262,
		MsgDefenderCombat:      8258,
		MsgDefenderCombatIdle1: 8259,
		MsgDefenderCombatIdle2: 8260,
		MsgDefenderReward:      8263,
	}
}
