// Package q084 is the quest Saga of the Ghost Sentinel: the third class
// change of a Phantom Ranger, on the saga core.
package q084

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/thirdclass"
)

// ghostSentinel is the class the saga leads to.
const ghostSentinel = 109

// New returns the saga, registered in f.
func New(f *thirdclass.Family) script.Script { return thirdclass.New(f, Saga()) }

// Saga returns what the saga sets on the core.
func Saga() thirdclass.Saga {
	return thirdclass.Saga{
		Name:    "Q084_SagaOfTheGhostSentinel",
		QuestID: 84,
		Title:   "Saga of the Ghost Sentinel",
		ClassID: ghostSentinel,

		ItemMain:        7107,
		ItemOptional:    0,
		ItemReward:      7521,
		ItemAmulet1st:   7282,
		ItemAmulet2nd:   7313,
		ItemAmulet3rd:   7344,
		ItemAmulet4th:   7375,
		ItemHalishaMark: 7499,
		ItemAmulet5th:   7406,
		ItemAmulet6th:   7437,

		NPCMain:      30702,
		NPC1st:       31604,
		NPC2nd:       31587,
		NPCTablet1st: 31646,
		NPCTablet2nd: 31649,
		NPCTablet3rd: 31652,
		NPC3rd:       31640,
		NPCTablet4th: 31654,
		NPC4th:       31641,
		NPCTablet5th: 31655,
		NPCTablet6th: 31659,
		NPCDefender:  31635,

		MobGuardian:  27216,
		MobCorrupted: 27298,
		MobHalisha:   27233,
		MobAttacker:  27307,

		LocCorrupted: script.Loc{X: 161719, Y: -92823, Z: -1893, Heading: -1},
		LocAttacker:  script.Loc{X: 124376, Y: 82127, Z: -2796, Heading: -1},
		LocDefender:  script.Loc{X: 124376, Y: 82127, Z: -2796, Heading: -1},

		MsgCorruptedSpawn:      8450,
		MsgCorruptedDespawn:    8451,
		MsgCorruptedKill:       8452,
		MsgHalishaSpawn:        8453,
		MsgHalishaDespawn:      8456,
		MsgHalishaKill:         8454,
		MsgHalishaKillOther:    8455,
		MsgAttackerSpawn:       8464,
		MsgAttackerDespawn:     8465,
		MsgAttackerAttack1:     8466,
		MsgAttackerAttack16:    8467,
		MsgDefenderSpawn:       8457,
		MsgDefenderDespawnWon:  8461,
		MsgDefenderDespawnLost: 8462,
		MsgDefenderCombat:      8458,
		MsgDefenderCombatIdle1: 8459,
		MsgDefenderCombatIdle2: 8460,
		MsgDefenderReward:      8463,
	}
}
