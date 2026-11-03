// Package q093 is the quest Saga of the Spectral Master: the third class
// change of a Phantom Summoner, on the saga core.
package q093

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/thirdclass"
)

// spectralMaster is the class the saga leads to.
const spectralMaster = 111

// New returns the saga, registered in f.
func New(f *thirdclass.Family) script.Script { return thirdclass.New(f, Saga()) }

// Saga returns what the saga sets on the core.
func Saga() thirdclass.Saga {
	return thirdclass.Saga{
		Name:    "Q093_SagaOfTheSpectralMaster",
		QuestID: 93,
		Title:   "Saga of the Spectral Master",
		ClassID: spectralMaster,

		ItemMain:        7112,
		ItemOptional:    0,
		ItemReward:      7606,
		ItemAmulet1st:   7291,
		ItemAmulet2nd:   7322,
		ItemAmulet3rd:   7353,
		ItemAmulet4th:   7384,
		ItemHalishaMark: 7508,
		ItemAmulet5th:   7415,
		ItemAmulet6th:   7446,

		NPCMain:      30175,
		NPC1st:       31613,
		NPC2nd:       31287,
		NPCTablet1st: 31646,
		NPCTablet2nd: 31649,
		NPCTablet3rd: 31653,
		NPC3rd:       30175,
		NPCTablet4th: 31654,
		NPC4th:       31613,
		NPCTablet5th: 31655,
		NPCTablet6th: 31656,
		NPCDefender:  31632,

		MobGuardian:  27216,
		MobCorrupted: 27315,
		MobHalisha:   27242,
		MobAttacker:  27312,

		LocCorrupted: script.Loc{X: 162898, Y: -76492, Z: -3096, Heading: -1},
		LocAttacker:  script.Loc{X: 47429, Y: -56923, Z: -2383, Heading: -1},
		LocDefender:  script.Loc{X: 47391, Y: -56929, Z: -2370, Heading: -1},

		MsgCorruptedSpawn:      9350,
		MsgCorruptedDespawn:    9351,
		MsgCorruptedKill:       9352,
		MsgHalishaSpawn:        9353,
		MsgHalishaDespawn:      9356,
		MsgHalishaKill:         9354,
		MsgHalishaKillOther:    9355,
		MsgAttackerSpawn:       9364,
		MsgAttackerDespawn:     9365,
		MsgAttackerAttack1:     9366,
		MsgAttackerAttack16:    9367,
		MsgDefenderSpawn:       9357,
		MsgDefenderDespawnWon:  9361,
		MsgDefenderDespawnLost: 9362,
		MsgDefenderCombat:      9358,
		MsgDefenderCombatIdle1: 9359,
		MsgDefenderCombatIdle2: 9360,
		MsgDefenderReward:      9363,
	}
}
