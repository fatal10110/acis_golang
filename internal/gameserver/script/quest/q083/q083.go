// Package q083 is the quest Saga of the Moonlight Sentinel: the third class
// change of a Silver Ranger, on the saga core.
package q083

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/thirdclass"
)

// moonlightSentinel is the class the saga leads to.
const moonlightSentinel = 102

// New returns the saga, registered in f.
func New(f *thirdclass.Family) script.Script { return thirdclass.New(f, Saga()) }

// Saga returns what the saga sets on the core.
func Saga() thirdclass.Saga {
	return thirdclass.Saga{
		Name:    "Q083_SagaOfTheMoonlightSentinel",
		QuestID: 83,
		Title:   "Saga of the Moonlight Sentinel",
		ClassID: moonlightSentinel,

		ItemMain:        7106,
		ItemOptional:    0,
		ItemReward:      7520,
		ItemAmulet1st:   7281,
		ItemAmulet2nd:   7312,
		ItemAmulet3rd:   7343,
		ItemAmulet4th:   7374,
		ItemHalishaMark: 7498,
		ItemAmulet5th:   7405,
		ItemAmulet6th:   7436,

		NPCMain:      30702,
		NPC1st:       31604,
		NPC2nd:       31627,
		NPCTablet1st: 31646,
		NPCTablet2nd: 31648,
		NPCTablet3rd: 31652,
		NPC3rd:       31640,
		NPCTablet4th: 31654,
		NPC4th:       31641,
		NPCTablet5th: 31655,
		NPCTablet6th: 31658,
		NPCDefender:  31634,

		MobGuardian:  27215,
		MobCorrupted: 27297,
		MobHalisha:   27232,
		MobAttacker:  27306,

		LocCorrupted: script.Loc{X: 161719, Y: -92823, Z: -1893, Heading: -1},
		LocAttacker:  script.Loc{X: 181227, Y: 36703, Z: -4816, Heading: -1},
		LocDefender:  script.Loc{X: 181215, Y: 36676, Z: -4812, Heading: -1},

		MsgCorruptedSpawn:      8350,
		MsgCorruptedDespawn:    8351,
		MsgCorruptedKill:       8352,
		MsgHalishaSpawn:        8353,
		MsgHalishaDespawn:      8356,
		MsgHalishaKill:         8354,
		MsgHalishaKillOther:    8355,
		MsgAttackerSpawn:       8364,
		MsgAttackerDespawn:     8365,
		MsgAttackerAttack1:     8366,
		MsgAttackerAttack16:    8367,
		MsgDefenderSpawn:       8357,
		MsgDefenderDespawnWon:  8361,
		MsgDefenderDespawnLost: 8362,
		MsgDefenderCombat:      8358,
		MsgDefenderCombatIdle1: 8359,
		MsgDefenderCombatIdle2: 8360,
		MsgDefenderReward:      8363,
	}
}
