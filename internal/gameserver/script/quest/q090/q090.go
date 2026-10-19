// Package q090 is the quest Saga of the Storm Screamer: the third class
// change of a Spellhowler, on the saga core.
package q090

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/thirdclass"
)

// stormScreamer is the class the saga leads to.
const stormScreamer = 110

// New returns the saga, registered in f.
func New(f *thirdclass.Family) script.Script { return thirdclass.New(f, Saga()) }

// Saga returns what the saga sets on the core.
func Saga() thirdclass.Saga {
	return thirdclass.Saga{
		Name:    "Q090_SagaOfTheStormScreamer",
		QuestID: 90,
		Title:   "Saga of the Storm Screamer",
		ClassID: stormScreamer,

		ItemMain:        7084,
		ItemOptional:    0,
		ItemReward:      7531,
		ItemAmulet1st:   7288,
		ItemAmulet2nd:   7319,
		ItemAmulet3rd:   7350,
		ItemAmulet4th:   7381,
		ItemHalishaMark: 7505,
		ItemAmulet5th:   7412,
		ItemAmulet6th:   7443,

		NPCMain:      30175,
		NPC1st:       31287,
		NPC2nd:       31627,
		NPCTablet1st: 31646,
		NPCTablet2nd: 31649,
		NPCTablet3rd: 31652,
		NPC3rd:       31287,
		NPCTablet4th: 31654,
		NPC4th:       31287,
		NPCTablet5th: 31655,
		NPCTablet6th: 31659,
		NPCDefender:  31598,

		MobGuardian:  27216,
		MobCorrupted: 27252,
		MobHalisha:   27239,
		MobAttacker:  27256,

		LocCorrupted: script.Loc{X: 161719, Y: -92823, Z: -1893, Heading: -1},
		LocAttacker:  script.Loc{X: 124376, Y: 82127, Z: -2796, Heading: -1},
		LocDefender:  script.Loc{X: 124355, Y: 82155, Z: -2803, Heading: -1},

		MsgCorruptedSpawn:      9050,
		MsgCorruptedDespawn:    9051,
		MsgCorruptedKill:       9052,
		MsgHalishaSpawn:        9053,
		MsgHalishaDespawn:      9056,
		MsgHalishaKill:         9054,
		MsgHalishaKillOther:    9055,
		MsgAttackerSpawn:       9064,
		MsgAttackerDespawn:     9065,
		MsgAttackerAttack1:     9066,
		MsgAttackerAttack16:    9067,
		MsgDefenderSpawn:       9057,
		MsgDefenderDespawnWon:  9061,
		MsgDefenderDespawnLost: 9062,
		MsgDefenderCombat:      9058,
		MsgDefenderCombatIdle1: 9059,
		MsgDefenderCombatIdle2: 9060,
		MsgDefenderReward:      9063,
	}
}
