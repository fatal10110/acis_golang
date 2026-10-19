// Package q100 is the quest Saga of the Maestro: the third class
// change of a Warsmith, on the saga core.
package q100

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/thirdclass"
)

// maestro is the class the saga leads to.
const maestro = 118

// New returns the saga, registered in f.
func New(f *thirdclass.Family) script.Script { return thirdclass.New(f, Saga()) }

// Saga returns what the saga sets on the core.
func Saga() thirdclass.Saga {
	return thirdclass.Saga{
		Name:    "Q100_SagaOfTheMaestro",
		QuestID: 100,
		Title:   "Saga of the Maestro",
		ClassID: maestro,

		ItemMain:        7108,
		ItemOptional:    0,
		ItemReward:      7607,
		ItemAmulet1st:   7298,
		ItemAmulet2nd:   7329,
		ItemAmulet3rd:   7360,
		ItemAmulet4th:   7391,
		ItemHalishaMark: 7515,
		ItemAmulet5th:   7422,
		ItemAmulet6th:   7453,

		NPCMain:      31592,
		NPC1st:       31597,
		NPC2nd:       31273,
		NPCTablet1st: 31646,
		NPCTablet2nd: 31648,
		NPCTablet3rd: 31653,
		NPC3rd:       31597,
		NPCTablet4th: 31654,
		NPC4th:       31597,
		NPCTablet5th: 31655,
		NPCTablet6th: 31656,
		NPCDefender:  31596,

		MobGuardian:  27215,
		MobCorrupted: 27260,
		MobHalisha:   27249,
		MobAttacker:  27308,

		LocCorrupted: script.Loc{X: 162898, Y: -76492, Z: -3096, Heading: -1},
		LocAttacker:  script.Loc{X: 47429, Y: -56923, Z: -2383, Heading: -1},
		LocDefender:  script.Loc{X: 47391, Y: -56929, Z: -2370, Heading: -1},

		MsgCorruptedSpawn:      10050,
		MsgCorruptedDespawn:    10051,
		MsgCorruptedKill:       10052,
		MsgHalishaSpawn:        10053,
		MsgHalishaDespawn:      10056,
		MsgHalishaKill:         10054,
		MsgHalishaKillOther:    10055,
		MsgAttackerSpawn:       10064,
		MsgAttackerDespawn:     10065,
		MsgAttackerAttack1:     10066,
		MsgAttackerAttack16:    10067,
		MsgDefenderSpawn:       10057,
		MsgDefenderDespawnWon:  10061,
		MsgDefenderDespawnLost: 10062,
		MsgDefenderCombat:      10058,
		MsgDefenderCombatIdle1: 10059,
		MsgDefenderCombatIdle2: 10060,
		MsgDefenderReward:      10063,
	}
}
