// Package q091 is the quest Saga of the Arcana Lord: the third class
// change of a Warlock, on the saga core.
package q091

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/thirdclass"
)

// arcanaLord is the class the saga leads to.
const arcanaLord = 96

// New returns the saga, registered in f.
func New(f *thirdclass.Family) script.Script { return thirdclass.New(f, Saga()) }

// Saga returns what the saga sets on the core.
func Saga() thirdclass.Saga {
	return thirdclass.Saga{
		Name:    "Q091_SagaOfTheArcanaLord",
		QuestID: 91,
		Title:   "Saga of the Arcana Lord",
		ClassID: arcanaLord,

		ItemMain:        7110,
		ItemOptional:    0,
		ItemReward:      7604,
		ItemAmulet1st:   7289,
		ItemAmulet2nd:   7320,
		ItemAmulet3rd:   7351,
		ItemAmulet4th:   7382,
		ItemHalishaMark: 7506,
		ItemAmulet5th:   7413,
		ItemAmulet6th:   7444,

		NPCMain:      31605,
		NPC1st:       31585,
		NPC2nd:       31622,
		NPCTablet1st: 31646,
		NPCTablet2nd: 31647,
		NPCTablet3rd: 31651,
		NPC3rd:       31608,
		NPCTablet4th: 31654,
		NPC4th:       31608,
		NPCTablet5th: 31655,
		NPCTablet6th: 31658,
		NPCDefender:  31586,

		MobGuardian:  27214,
		MobCorrupted: 27313,
		MobHalisha:   27240,
		MobAttacker:  27310,

		LocCorrupted: script.Loc{X: 119518, Y: -28658, Z: -3811, Heading: -1},
		LocAttacker:  script.Loc{X: 181215, Y: 36676, Z: -4812, Heading: -1},
		LocDefender:  script.Loc{X: 181227, Y: 36703, Z: -4816, Heading: -1},

		MsgCorruptedSpawn:      9150,
		MsgCorruptedDespawn:    9151,
		MsgCorruptedKill:       9152,
		MsgHalishaSpawn:        9153,
		MsgHalishaDespawn:      9156,
		MsgHalishaKill:         9154,
		MsgHalishaKillOther:    9155,
		MsgAttackerSpawn:       9164,
		MsgAttackerDespawn:     9165,
		MsgAttackerAttack1:     9166,
		MsgAttackerAttack16:    9167,
		MsgDefenderSpawn:       9157,
		MsgDefenderDespawnWon:  9161,
		MsgDefenderDespawnLost: 9162,
		MsgDefenderCombat:      9158,
		MsgDefenderCombatIdle1: 9159,
		MsgDefenderCombatIdle2: 9160,
		MsgDefenderReward:      9163,
	}
}
