// Package q096 is the quest Saga of the Spectral Dancer: the third class
// change of a Bladedancer, on the saga core.
package q096

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/thirdclass"
)

// spectralDancer is the class the saga leads to.
const spectralDancer = 107

// New returns the saga, registered in f.
func New(f *thirdclass.Family) script.Script { return thirdclass.New(f, Saga()) }

// Saga returns what the saga sets on the core.
func Saga() thirdclass.Saga {
	return thirdclass.Saga{
		Name:    "Q096_SagaOfTheSpectralDancer",
		QuestID: 96,
		Title:   "Saga of the Spectral Dancer",
		ClassID: spectralDancer,

		ItemMain:        7092,
		ItemOptional:    0,
		ItemReward:      7527,
		ItemAmulet1st:   7294,
		ItemAmulet2nd:   7325,
		ItemAmulet3rd:   7356,
		ItemAmulet4th:   7387,
		ItemHalishaMark: 7511,
		ItemAmulet5th:   7418,
		ItemAmulet6th:   7449,

		NPCMain:      31582,
		NPC1st:       31284,
		NPC2nd:       31623,
		NPCTablet1st: 31646,
		NPCTablet2nd: 31649,
		NPCTablet3rd: 31653,
		NPC3rd:       31284,
		NPCTablet4th: 31654,
		NPC4th:       31284,
		NPCTablet5th: 31655,
		NPCTablet6th: 31656,
		NPCDefender:  31611,

		MobGuardian:  27216,
		MobCorrupted: 27272,
		MobHalisha:   27245,
		MobAttacker:  27264,

		LocCorrupted: script.Loc{X: 162898, Y: -76492, Z: -3096, Heading: -1},
		LocAttacker:  script.Loc{X: 47429, Y: -56923, Z: -2383, Heading: -1},
		LocDefender:  script.Loc{X: 47391, Y: -56929, Z: -2370, Heading: -1},

		MsgCorruptedSpawn:      9650,
		MsgCorruptedDespawn:    9651,
		MsgCorruptedKill:       9652,
		MsgHalishaSpawn:        9653,
		MsgHalishaDespawn:      9656,
		MsgHalishaKill:         9654,
		MsgHalishaKillOther:    9655,
		MsgAttackerSpawn:       9664,
		MsgAttackerDespawn:     9665,
		MsgAttackerAttack1:     9666,
		MsgAttackerAttack16:    9667,
		MsgDefenderSpawn:       9657,
		MsgDefenderDespawnWon:  9661,
		MsgDefenderDespawnLost: 9662,
		MsgDefenderCombat:      9658,
		MsgDefenderCombatIdle1: 9659,
		MsgDefenderCombatIdle2: 9660,
		MsgDefenderReward:      9663,
	}
}
