// Package q092 is the quest Saga of the Elemental Master: the third class
// change of an Elemental Summoner, on the saga core.
package q092

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/thirdclass"
)

// elementalMaster is the class the saga leads to.
const elementalMaster = 104

// New returns the saga, registered in f.
func New(f *thirdclass.Family) script.Script { return thirdclass.New(f, Saga()) }

// Saga returns what the saga sets on the core.
func Saga() thirdclass.Saga {
	return thirdclass.Saga{
		Name:    "Q092_SagaOfTheElementalMaster",
		QuestID: 92,
		Title:   "Saga of the Elemental Master",
		ClassID: elementalMaster,

		ItemMain:        7111,
		ItemOptional:    0,
		ItemReward:      7605,
		ItemAmulet1st:   7290,
		ItemAmulet2nd:   7321,
		ItemAmulet3rd:   7352,
		ItemAmulet4th:   7383,
		ItemHalishaMark: 7507,
		ItemAmulet5th:   7414,
		ItemAmulet6th:   7445,

		NPCMain:      30174,
		NPC1st:       31614,
		NPC2nd:       31281,
		NPCTablet1st: 31646,
		NPCTablet2nd: 31648,
		NPCTablet3rd: 31652,
		NPC3rd:       31614,
		NPCTablet4th: 31654,
		NPC4th:       31614,
		NPCTablet5th: 31655,
		NPCTablet6th: 31659,
		NPCDefender:  31629,

		MobGuardian:  27215,
		MobCorrupted: 27314,
		MobHalisha:   27241,
		MobAttacker:  27311,

		LocCorrupted: script.Loc{X: 161719, Y: -92823, Z: -1893, Heading: -1},
		LocAttacker:  script.Loc{X: 124376, Y: 82127, Z: -2796, Heading: -1},
		LocDefender:  script.Loc{X: 124355, Y: 82155, Z: -2803, Heading: -1},

		MsgCorruptedSpawn:      9250,
		MsgCorruptedDespawn:    9251,
		MsgCorruptedKill:       9252,
		MsgHalishaSpawn:        9253,
		MsgHalishaDespawn:      9256,
		MsgHalishaKill:         9254,
		MsgHalishaKillOther:    9255,
		MsgAttackerSpawn:       9264,
		MsgAttackerDespawn:     9265,
		MsgAttackerAttack1:     9266,
		MsgAttackerAttack16:    9267,
		MsgDefenderSpawn:       9257,
		MsgDefenderDespawnWon:  9261,
		MsgDefenderDespawnLost: 9262,
		MsgDefenderCombat:      9258,
		MsgDefenderCombatIdle1: 9259,
		MsgDefenderCombatIdle2: 9260,
		MsgDefenderReward:      9263,
	}
}
