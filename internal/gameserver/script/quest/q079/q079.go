// Package q079 is the quest Saga of the Adventurer: the third class
// change of a Treasure Hunter, on the saga core.
package q079

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/thirdclass"
)

// adventurer is the class the saga leads to.
const adventurer = 93

// New returns the saga, registered in f.
func New(f *thirdclass.Family) script.Script { return thirdclass.New(f, Saga()) }

// Saga returns what the saga sets on the core.
func Saga() thirdclass.Saga {
	return thirdclass.Saga{
		Name:    "Q079_SagaOfTheAdventurer",
		QuestID: 79,
		Title:   "Saga of the Adventurer",
		ClassID: adventurer,

		ItemMain:        7102,
		ItemOptional:    0,
		ItemReward:      7516,
		ItemAmulet1st:   7277,
		ItemAmulet2nd:   7308,
		ItemAmulet3rd:   7339,
		ItemAmulet4th:   7370,
		ItemHalishaMark: 7494,
		ItemAmulet5th:   7401,
		ItemAmulet6th:   7432,

		NPCMain:      31603,
		NPC1st:       31579,
		NPC2nd:       31584,
		NPCTablet1st: 31646,
		NPCTablet2nd: 31647,
		NPCTablet3rd: 31651,
		NPC3rd:       31615,
		NPCTablet4th: 31654,
		NPC4th:       31616,
		NPCTablet5th: 31655,
		NPCTablet6th: 31658,
		NPCDefender:  31619,

		MobGuardian:  27214,
		MobCorrupted: 27299,
		MobHalisha:   27228,
		MobAttacker:  27302,

		LocCorrupted: script.Loc{X: 119518, Y: -28658, Z: -3811, Heading: -1},
		LocAttacker:  script.Loc{X: 181205, Y: 36676, Z: -4816, Heading: -1},
		LocDefender:  script.Loc{X: 181215, Y: 36676, Z: -4812, Heading: -1},

		MsgCorruptedSpawn:      7950,
		MsgCorruptedDespawn:    7951,
		MsgCorruptedKill:       7952,
		MsgHalishaSpawn:        7953,
		MsgHalishaDespawn:      7956,
		MsgHalishaKill:         7954,
		MsgHalishaKillOther:    7955,
		MsgAttackerSpawn:       7964,
		MsgAttackerDespawn:     7965,
		MsgAttackerAttack1:     7966,
		MsgAttackerAttack16:    7967,
		MsgDefenderSpawn:       7957,
		MsgDefenderDespawnWon:  7961,
		MsgDefenderDespawnLost: 7962,
		MsgDefenderCombat:      7958,
		MsgDefenderCombatIdle1: 7959,
		MsgDefenderCombatIdle2: 7960,
		MsgDefenderReward:      7963,
	}
}
