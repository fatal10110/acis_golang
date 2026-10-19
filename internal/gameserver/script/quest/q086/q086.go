// Package q086 is the quest Saga of the Hierophant: the third class
// change of a Prophet, on the saga core.
package q086

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/thirdclass"
)

// hierophant is the class the saga leads to.
const hierophant = 98

// New returns the saga, registered in f.
func New(f *thirdclass.Family) script.Script { return thirdclass.New(f, Saga()) }

// Saga returns what the saga sets on the core.
func Saga() thirdclass.Saga {
	return thirdclass.Saga{
		Name:    "Q086_SagaOfTheHierophant",
		QuestID: 86,
		Title:   "Saga of the Hierophant",
		ClassID: hierophant,

		ItemMain:        7089,
		ItemOptional:    0,
		ItemReward:      7523,
		ItemAmulet1st:   7284,
		ItemAmulet2nd:   7315,
		ItemAmulet3rd:   7346,
		ItemAmulet4th:   7377,
		ItemHalishaMark: 7501,
		ItemAmulet5th:   7408,
		ItemAmulet6th:   7439,

		NPCMain:      30191,
		NPC1st:       31588,
		NPC2nd:       31626,
		NPCTablet1st: 31646,
		NPCTablet2nd: 31648,
		NPCTablet3rd: 31652,
		NPC3rd:       31280,
		NPCTablet4th: 31654,
		NPC4th:       31280,
		NPCTablet5th: 31655,
		NPCTablet6th: 31659,
		NPCDefender:  31591,

		MobGuardian:  27215,
		MobCorrupted: 27269,
		MobHalisha:   27235,
		MobAttacker:  27275,

		LocCorrupted: script.Loc{X: 161719, Y: -92823, Z: -1893, Heading: -1},
		LocAttacker:  script.Loc{X: 124355, Y: 82155, Z: -2803, Heading: -1},
		LocDefender:  script.Loc{X: 124376, Y: 82127, Z: -2796, Heading: -1},

		MsgCorruptedSpawn:      8650,
		MsgCorruptedDespawn:    8651,
		MsgCorruptedKill:       8652,
		MsgHalishaSpawn:        8653,
		MsgHalishaDespawn:      8656,
		MsgHalishaKill:         8654,
		MsgHalishaKillOther:    8655,
		MsgAttackerSpawn:       8664,
		MsgAttackerDespawn:     8665,
		MsgAttackerAttack1:     8666,
		MsgAttackerAttack16:    8667,
		MsgDefenderSpawn:       8657,
		MsgDefenderDespawnWon:  8661,
		MsgDefenderDespawnLost: 8662,
		MsgDefenderCombat:      8658,
		MsgDefenderCombatIdle1: 8659,
		MsgDefenderCombatIdle2: 8660,
		MsgDefenderReward:      8663,
	}
}
