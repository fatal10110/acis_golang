// Package q074 is the quest Saga of the Dreadnought: the third class
// change of a Warlord, on the saga core.
package q074

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/thirdclass"
)

// dreadnought is the class the saga leads to.
const dreadnought = 89

// New returns the saga, registered in f.
func New(f *thirdclass.Family) script.Script { return thirdclass.New(f, Saga()) }

// Saga returns what the saga sets on the core.
func Saga() thirdclass.Saga {
	return thirdclass.Saga{
		Name:    "Q074_SagaOfTheDreadnought",
		QuestID: 74,
		Title:   "Saga of the Dreadnought",
		ClassID: dreadnought,

		ItemMain:        7097,
		ItemOptional:    6480,
		ItemReward:      7538,
		ItemAmulet1st:   7272,
		ItemAmulet2nd:   7303,
		ItemAmulet3rd:   7334,
		ItemAmulet4th:   7365,
		ItemHalishaMark: 7489,
		ItemAmulet5th:   7396,
		ItemAmulet6th:   7427,

		NPCMain:      30850,
		NPC1st:       31298,
		NPC2nd:       31624,
		NPCTablet1st: 31646,
		NPCTablet2nd: 31648,
		NPCTablet3rd: 31650,
		NPC3rd:       31276,
		NPCTablet4th: 31654,
		NPC4th:       31522,
		NPCTablet5th: 31655,
		NPCTablet6th: 31657,
		NPCDefender:  31595,

		MobGuardian:  27215,
		MobCorrupted: 27290,
		MobHalisha:   27223,
		MobAttacker:  27282,

		LocCorrupted: script.Loc{X: 191046, Y: -40640, Z: -3042, Heading: -1},
		LocAttacker:  script.Loc{X: 46087, Y: -36372, Z: -1685, Heading: -1},
		LocDefender:  script.Loc{X: 46066, Y: -36396, Z: -1685, Heading: -1},

		MsgCorruptedSpawn:      7450,
		MsgCorruptedDespawn:    7451,
		MsgCorruptedKill:       7452,
		MsgHalishaSpawn:        7453,
		MsgHalishaDespawn:      7456,
		MsgHalishaKill:         7454,
		MsgHalishaKillOther:    7455,
		MsgAttackerSpawn:       7464,
		MsgAttackerDespawn:     7465,
		MsgAttackerAttack1:     7466,
		MsgAttackerAttack16:    7467,
		MsgDefenderSpawn:       7457,
		MsgDefenderDespawnWon:  7461,
		MsgDefenderDespawnLost: 7462,
		MsgDefenderCombat:      7458,
		MsgDefenderCombatIdle1: 7459,
		MsgDefenderCombatIdle2: 7460,
		MsgDefenderReward:      7463,
	}
}
