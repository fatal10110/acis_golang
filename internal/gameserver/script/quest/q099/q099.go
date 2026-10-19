// Package q099 is the quest Saga of the Fortune Seeker: the third class
// change of a Bounty Hunter, on the saga core.
package q099

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/thirdclass"
)

// fortuneSeeker is the class the saga leads to.
const fortuneSeeker = 117

// New returns the saga, registered in f.
func New(f *thirdclass.Family) script.Script { return thirdclass.New(f, Saga()) }

// Saga returns what the saga sets on the core.
func Saga() thirdclass.Saga {
	return thirdclass.Saga{
		Name:    "Q099_SagaOfTheFortuneSeeker",
		QuestID: 99,
		Title:   "Saga of the Fortune Seeker",
		ClassID: fortuneSeeker,

		ItemMain:        7109,
		ItemOptional:    0,
		ItemReward:      7608,
		ItemAmulet1st:   7297,
		ItemAmulet2nd:   7328,
		ItemAmulet3rd:   7359,
		ItemAmulet4th:   7390,
		ItemHalishaMark: 7514,
		ItemAmulet5th:   7421,
		ItemAmulet6th:   7452,

		NPCMain:      31594,
		NPC1st:       31600,
		NPC2nd:       31623,
		NPCTablet1st: 31646,
		NPCTablet2nd: 31649,
		NPCTablet3rd: 31650,
		NPC3rd:       31600,
		NPCTablet4th: 31654,
		NPC4th:       31600,
		NPCTablet5th: 31655,
		NPCTablet6th: 31657,
		NPCDefender:  31601,

		MobGuardian:  27216,
		MobCorrupted: 27259,
		MobHalisha:   27248,
		MobAttacker:  27309,

		LocCorrupted: script.Loc{X: 191046, Y: -40640, Z: -3042, Heading: -1},
		LocAttacker:  script.Loc{X: 46066, Y: -36396, Z: -1685, Heading: -1},
		LocDefender:  script.Loc{X: 46087, Y: -36372, Z: -1685, Heading: -1},

		MsgCorruptedSpawn:      9950,
		MsgCorruptedDespawn:    9951,
		MsgCorruptedKill:       9952,
		MsgHalishaSpawn:        9953,
		MsgHalishaDespawn:      9956,
		MsgHalishaKill:         9954,
		MsgHalishaKillOther:    9955,
		MsgAttackerSpawn:       9964,
		MsgAttackerDespawn:     9965,
		MsgAttackerAttack1:     9966,
		MsgAttackerAttack16:    9967,
		MsgDefenderSpawn:       9957,
		MsgDefenderDespawnWon:  9961,
		MsgDefenderDespawnLost: 9962,
		MsgDefenderCombat:      9958,
		MsgDefenderCombatIdle1: 9959,
		MsgDefenderCombatIdle2: 9960,
		MsgDefenderReward:      9963,
	}
}
