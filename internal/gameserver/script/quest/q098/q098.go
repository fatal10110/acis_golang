// Package q098 is the quest Saga of the Shillien Saint: the third class
// change of a Shillien Elder, on the saga core.
package q098

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/thirdclass"
)

// shillienSaint is the class the saga leads to.
const shillienSaint = 112

// New returns the saga, registered in f.
func New(f *thirdclass.Family) script.Script { return thirdclass.New(f, Saga()) }

// Saga returns what the saga sets on the core.
func Saga() thirdclass.Saga {
	return thirdclass.Saga{
		Name:    "Q098_SagaOfTheShillienSaint",
		QuestID: 98,
		Title:   "Saga of the Shillien Saint",
		ClassID: shillienSaint,

		ItemMain:        7090,
		ItemOptional:    0,
		ItemReward:      7525,
		ItemAmulet1st:   7296,
		ItemAmulet2nd:   7327,
		ItemAmulet3rd:   7358,
		ItemAmulet4th:   7389,
		ItemHalishaMark: 7513,
		ItemAmulet5th:   7420,
		ItemAmulet6th:   7451,

		NPCMain:      31581,
		NPC1st:       31588,
		NPC2nd:       31626,
		NPCTablet1st: 31646,
		NPCTablet2nd: 31647,
		NPCTablet3rd: 31651,
		NPC3rd:       31287,
		NPCTablet4th: 31654,
		NPC4th:       31287,
		NPCTablet5th: 31655,
		NPCTablet6th: 31658,
		NPCDefender:  31621,

		MobGuardian:  27214,
		MobCorrupted: 27270,
		MobHalisha:   27247,
		MobAttacker:  27277,

		LocCorrupted: script.Loc{X: 119518, Y: -28658, Z: -3811, Heading: -1},
		LocAttacker:  script.Loc{X: 181215, Y: 36676, Z: -4812, Heading: -1},
		LocDefender:  script.Loc{X: 181227, Y: 36703, Z: -4816, Heading: -1},

		MsgCorruptedSpawn:      9850,
		MsgCorruptedDespawn:    9851,
		MsgCorruptedKill:       9852,
		MsgHalishaSpawn:        9853,
		MsgHalishaDespawn:      9856,
		MsgHalishaKill:         9854,
		MsgHalishaKillOther:    9855,
		MsgAttackerSpawn:       9864,
		MsgAttackerDespawn:     9865,
		MsgAttackerAttack1:     9866,
		MsgAttackerAttack16:    9867,
		MsgDefenderSpawn:       9857,
		MsgDefenderDespawnWon:  9861,
		MsgDefenderDespawnLost: 9862,
		MsgDefenderCombat:      9858,
		MsgDefenderCombatIdle1: 9859,
		MsgDefenderCombatIdle2: 9860,
		MsgDefenderReward:      9863,
	}
}
