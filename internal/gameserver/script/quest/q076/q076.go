// Package q076 is the quest Saga of the Grand Khavatari: the third class
// change of a Tyrant, on the saga core.
package q076

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/thirdclass"
)

// grandKhavatari is the class the saga leads to.
const grandKhavatari = 114

// New returns the saga, registered in f.
func New(f *thirdclass.Family) script.Script { return thirdclass.New(f, Saga()) }

// Saga returns what the saga sets on the core.
func Saga() thirdclass.Saga {
	return thirdclass.Saga{
		Name:    "Q076_SagaOfTheGrandKhavatari",
		QuestID: 76,
		Title:   "Saga of the Grand Khavatari",
		ClassID: grandKhavatari,

		ItemMain:        7099,
		ItemOptional:    0,
		ItemReward:      7539,
		ItemAmulet1st:   7274,
		ItemAmulet2nd:   7305,
		ItemAmulet3rd:   7336,
		ItemAmulet4th:   7367,
		ItemHalishaMark: 7491,
		ItemAmulet5th:   7398,
		ItemAmulet6th:   7429,

		NPCMain:      31339,
		NPC1st:       31589,
		NPC2nd:       31624,
		NPCTablet1st: 31646,
		NPCTablet2nd: 31647,
		NPCTablet3rd: 31652,
		NPC3rd:       31290,
		NPCTablet4th: 31654,
		NPC4th:       31290,
		NPCTablet5th: 31655,
		NPCTablet6th: 31659,
		NPCDefender:  31637,

		MobGuardian:  27214,
		MobCorrupted: 27293,
		MobHalisha:   27226,
		MobAttacker:  27284,

		LocCorrupted: script.Loc{X: 161719, Y: -92823, Z: -1893, Heading: -1},
		LocAttacker:  script.Loc{X: 124355, Y: 82155, Z: -2803, Heading: -1},
		LocDefender:  script.Loc{X: 124376, Y: 82127, Z: -2796, Heading: -1},

		MsgCorruptedSpawn:      7650,
		MsgCorruptedDespawn:    7651,
		MsgCorruptedKill:       7652,
		MsgHalishaSpawn:        7653,
		MsgHalishaDespawn:      7656,
		MsgHalishaKill:         7654,
		MsgHalishaKillOther:    7655,
		MsgAttackerSpawn:       7664,
		MsgAttackerDespawn:     7665,
		MsgAttackerAttack1:     7666,
		MsgAttackerAttack16:    7667,
		MsgDefenderSpawn:       7657,
		MsgDefenderDespawnWon:  7661,
		MsgDefenderDespawnLost: 7662,
		MsgDefenderCombat:      7658,
		MsgDefenderCombatIdle1: 7659,
		MsgDefenderCombatIdle2: 7660,
		MsgDefenderReward:      7663,
	}
}
