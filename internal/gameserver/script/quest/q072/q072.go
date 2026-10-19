// Package q072 is the quest Saga of the Sword Muse: the third class
// change of a Sword Singer, on the saga core.
package q072

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/thirdclass"
)

// swordMuse is the class the saga leads to.
const swordMuse = 100

// New returns the saga, registered in f.
func New(f *thirdclass.Family) script.Script { return thirdclass.New(f, Saga()) }

// Saga returns what the saga sets on the core.
func Saga() thirdclass.Saga {
	return thirdclass.Saga{
		Name:    "Q072_SagaOfTheSwordMuse",
		QuestID: 72,
		Title:   "Saga of the Sword Muse",
		ClassID: swordMuse,

		ItemMain:        7095,
		ItemOptional:    6482,
		ItemReward:      7536,
		ItemAmulet1st:   7270,
		ItemAmulet2nd:   7301,
		ItemAmulet3rd:   7332,
		ItemAmulet4th:   7363,
		ItemHalishaMark: 7487,
		ItemAmulet5th:   7394,
		ItemAmulet6th:   7425,

		NPCMain:      30853,
		NPC1st:       31583,
		NPC2nd:       31624,
		NPCTablet1st: 31646,
		NPCTablet2nd: 31649,
		NPCTablet3rd: 31652,
		NPC3rd:       31537,
		NPCTablet4th: 31654,
		NPC4th:       31281,
		NPCTablet5th: 31655,
		NPCTablet6th: 31659,
		NPCDefender:  31618,

		MobGuardian:  27216,
		MobCorrupted: 27288,
		MobHalisha:   27221,
		MobAttacker:  27280,

		LocCorrupted: script.Loc{X: 161719, Y: -92823, Z: -1893, Heading: -1},
		LocAttacker:  script.Loc{X: 124355, Y: 82155, Z: -2803, Heading: -1},
		LocDefender:  script.Loc{X: 124376, Y: 82127, Z: -2796, Heading: -1},

		MsgCorruptedSpawn:      7250,
		MsgCorruptedDespawn:    7251,
		MsgCorruptedKill:       7252,
		MsgHalishaSpawn:        7253,
		MsgHalishaDespawn:      7256,
		MsgHalishaKill:         7254,
		MsgHalishaKillOther:    7255,
		MsgAttackerSpawn:       7264,
		MsgAttackerDespawn:     7265,
		MsgAttackerAttack1:     7266,
		MsgAttackerAttack16:    7267,
		MsgDefenderSpawn:       7257,
		MsgDefenderDespawnWon:  7261,
		MsgDefenderDespawnLost: 7262,
		MsgDefenderCombat:      7258,
		MsgDefenderCombatIdle1: 7259,
		MsgDefenderCombatIdle2: 7260,
		MsgDefenderReward:      7263,
	}
}
