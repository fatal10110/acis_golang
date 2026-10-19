// Package q088 is the quest Saga of the Archmage: the third class
// change of a Sorcerer, on the saga core.
package q088

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/thirdclass"
)

// archmage is the class the saga leads to.
const archmage = 94

// New returns the saga, registered in f.
func New(f *thirdclass.Family) script.Script { return thirdclass.New(f, Saga()) }

// Saga returns what the saga sets on the core.
func Saga() thirdclass.Saga {
	return thirdclass.Saga{
		Name:    "Q088_SagaOfTheArchmage",
		QuestID: 88,
		Title:   "Saga of the Archmage",
		ClassID: archmage,

		ItemMain:        7082,
		ItemOptional:    0,
		ItemReward:      7529,
		ItemAmulet1st:   7286,
		ItemAmulet2nd:   7317,
		ItemAmulet3rd:   7348,
		ItemAmulet4th:   7379,
		ItemHalishaMark: 7503,
		ItemAmulet5th:   7410,
		ItemAmulet6th:   7441,

		NPCMain:      30176,
		NPC1st:       31282,
		NPC2nd:       31627,
		NPCTablet1st: 31646,
		NPCTablet2nd: 31647,
		NPCTablet3rd: 31650,
		NPC3rd:       31282,
		NPCTablet4th: 31654,
		NPC4th:       31282,
		NPCTablet5th: 31655,
		NPCTablet6th: 31657,
		NPCDefender:  31590,

		MobGuardian:  27214,
		MobCorrupted: 27250,
		MobHalisha:   27237,
		MobAttacker:  27254,

		LocCorrupted: script.Loc{X: 191046, Y: -40640, Z: -3042, Heading: -1},
		LocAttacker:  script.Loc{X: 46066, Y: -36396, Z: -1685, Heading: -1},
		LocDefender:  script.Loc{X: 46087, Y: -36372, Z: -1685, Heading: -1},

		MsgCorruptedSpawn:      8850,
		MsgCorruptedDespawn:    8851,
		MsgCorruptedKill:       8852,
		MsgHalishaSpawn:        8853,
		MsgHalishaDespawn:      8856,
		MsgHalishaKill:         8854,
		MsgHalishaKillOther:    8855,
		MsgAttackerSpawn:       8864,
		MsgAttackerDespawn:     8865,
		MsgAttackerAttack1:     8866,
		MsgAttackerAttack16:    8867,
		MsgDefenderSpawn:       8857,
		MsgDefenderDespawnWon:  8861,
		MsgDefenderDespawnLost: 8862,
		MsgDefenderCombat:      8858,
		MsgDefenderCombatIdle1: 8859,
		MsgDefenderCombatIdle2: 8860,
		MsgDefenderReward:      8863,
	}
}
