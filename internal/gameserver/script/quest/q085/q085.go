// Package q085 is the quest Saga of the Cardinal: the third class
// change of a Bishop, on the saga core.
package q085

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/thirdclass"
)

// cardinal is the class the saga leads to.
const cardinal = 97

// New returns the saga, registered in f.
func New(f *thirdclass.Family) script.Script { return thirdclass.New(f, Saga()) }

// Saga returns what the saga sets on the core.
func Saga() thirdclass.Saga {
	return thirdclass.Saga{
		Name:    "Q085_SagaOfTheCardinal",
		QuestID: 85,
		Title:   "Saga of the Cardinal",
		ClassID: cardinal,

		ItemMain:        7087,
		ItemOptional:    0,
		ItemReward:      7522,
		ItemAmulet1st:   7283,
		ItemAmulet2nd:   7314,
		ItemAmulet3rd:   7345,
		ItemAmulet4th:   7376,
		ItemHalishaMark: 7500,
		ItemAmulet5th:   7407,
		ItemAmulet6th:   7438,

		NPCMain:      30191,
		NPC1st:       31588,
		NPC2nd:       31626,
		NPCTablet1st: 31646,
		NPCTablet2nd: 31647,
		NPCTablet3rd: 31651,
		NPC3rd:       31280,
		NPCTablet4th: 31654,
		NPC4th:       31280,
		NPCTablet5th: 31655,
		NPCTablet6th: 31658,
		NPCDefender:  31644,

		MobGuardian:  27214,
		MobCorrupted: 27267,
		MobHalisha:   27234,
		MobAttacker:  27274,

		LocCorrupted: script.Loc{X: 119518, Y: -28658, Z: -3811, Heading: -1},
		LocAttacker:  script.Loc{X: 181215, Y: 36676, Z: -4812, Heading: -1},
		LocDefender:  script.Loc{X: 181227, Y: 36703, Z: -4816, Heading: -1},

		MsgCorruptedSpawn:      8550,
		MsgCorruptedDespawn:    8551,
		MsgCorruptedKill:       8552,
		MsgHalishaSpawn:        8553,
		MsgHalishaDespawn:      8556,
		MsgHalishaKill:         8554,
		MsgHalishaKillOther:    8555,
		MsgAttackerSpawn:       8564,
		MsgAttackerDespawn:     8565,
		MsgAttackerAttack1:     8566,
		MsgAttackerAttack16:    8567,
		MsgDefenderSpawn:       8557,
		MsgDefenderDespawnWon:  8561,
		MsgDefenderDespawnLost: 8562,
		MsgDefenderCombat:      8558,
		MsgDefenderCombatIdle1: 8559,
		MsgDefenderCombatIdle2: 8560,
		MsgDefenderReward:      8563,
	}
}
