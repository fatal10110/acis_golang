// Package q081 is the quest Saga of the Ghost Hunter: the third class
// change of an Abyss Walker, on the saga core.
package q081

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/thirdclass"
)

// ghostHunter is the class the saga leads to.
const ghostHunter = 108

// New returns the saga, registered in f.
func New(f *thirdclass.Family) script.Script { return thirdclass.New(f, Saga()) }

// Saga returns what the saga sets on the core.
func Saga() thirdclass.Saga {
	return thirdclass.Saga{
		Name:    "Q081_SagaOfTheGhostHunter",
		QuestID: 81,
		Title:   "Saga of the Ghost Hunter",
		ClassID: ghostHunter,

		ItemMain:        7104,
		ItemOptional:    0,
		ItemReward:      7518,
		ItemAmulet1st:   7279,
		ItemAmulet2nd:   7310,
		ItemAmulet3rd:   7341,
		ItemAmulet4th:   7372,
		ItemHalishaMark: 7496,
		ItemAmulet5th:   7403,
		ItemAmulet6th:   7434,

		NPCMain:      31603,
		NPC1st:       31286,
		NPC2nd:       31624,
		NPCTablet1st: 31646,
		NPCTablet2nd: 31649,
		NPCTablet3rd: 31653,
		NPC3rd:       31615,
		NPCTablet4th: 31654,
		NPC4th:       31616,
		NPCTablet5th: 31655,
		NPCTablet6th: 31656,
		NPCDefender:  31617,

		MobGuardian:  27216,
		MobCorrupted: 27301,
		MobHalisha:   27230,
		MobAttacker:  27304,

		LocCorrupted: script.Loc{X: 162898, Y: -76492, Z: -3096, Heading: -1},
		LocAttacker:  script.Loc{X: 47391, Y: -56929, Z: -2370, Heading: -1},
		LocDefender:  script.Loc{X: 47429, Y: -56923, Z: -2383, Heading: -1},

		MsgCorruptedSpawn:      8150,
		MsgCorruptedDespawn:    8151,
		MsgCorruptedKill:       8152,
		MsgHalishaSpawn:        8153,
		MsgHalishaDespawn:      8156,
		MsgHalishaKill:         8154,
		MsgHalishaKillOther:    8155,
		MsgAttackerSpawn:       8164,
		MsgAttackerDespawn:     8165,
		MsgAttackerAttack1:     8166,
		MsgAttackerAttack16:    8167,
		MsgDefenderSpawn:       8157,
		MsgDefenderDespawnWon:  8161,
		MsgDefenderDespawnLost: 8162,
		MsgDefenderCombat:      8158,
		MsgDefenderCombatIdle1: 8159,
		MsgDefenderCombatIdle2: 8160,
		MsgDefenderReward:      8163,
	}
}
